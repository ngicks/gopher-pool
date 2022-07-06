package gopherpool

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

var (
	ErrAlreadyStarted = errors.New("already started")
	ErrAlreadyEnded   = errors.New("already ended")
)

// WorkerConstructor is aliased type of constructor.
// id must be unique value. Overlapping id causes undefined behavior.
// onTaskReceived, onTaskDone can be nil.
type WorkerConstructor = func(id int, onTaskReceived func(), onTaskDone func()) *Worker[int]

// BuildWorkerConstructor is helper function for WorkerConstructor.
// taskCh must not be nil. onTaskReceived_, onTaskDone_ can be nil.
func BuildWorkerConstructor(workCh <-chan WorkFn, onTaskReceived_ func(), onTaskDone_ func()) WorkerConstructor {
	return func(id int, onTaskReceived__ func(), onTaskDone__ func()) *Worker[int] {
		onTaskReceived := func() {
			if onTaskReceived_ != nil {
				onTaskReceived_()
			}
			if onTaskReceived__ != nil {
				onTaskReceived__()
			}
		}
		onTaskDone := func() {
			if onTaskDone_ != nil {
				onTaskDone_()
			}
			if onTaskDone__ != nil {
				onTaskDone__()
			}
		}
		w, err := NewWorker(id, workCh, onTaskReceived, onTaskDone)
		if err != nil {
			panic(err)
		}
		return w
	}
}

// WorkerPool is container for workers.
type WorkerPool struct {
	mu sync.RWMutex
	wg sync.WaitGroup

	activeWorkerNum int64
	workCh          chan WorkFn

	workerConstructor WorkerConstructor
	workerIdx         int
	workers           map[int]*Worker[int]
	sleepingWorkers   map[int]*Worker[int]

	abnormalReturnCb func(error)
}

func NewWorkerPool(
	options ...Option,
) *WorkerPool {
	w := &WorkerPool{
		workers:          make(map[int]*Worker[int], 0),
		sleepingWorkers:  make(map[int]*Worker[int], 0),
		abnormalReturnCb: func(err error) {},
	}

	for _, opt := range options {
		w = opt(w)
	}

	if w.workCh == nil {
		workCh := make(chan WorkFn)
		w.workCh = workCh
		w.workerConstructor = BuildWorkerConstructor(workCh, nil, nil)
	}

	return w
}

// SenderChan is getter of sender side of WorkFn chan.
func (p *WorkerPool) SenderChan() chan<- WorkFn {
	return p.workCh
}

// Send is wrapper that sends workFn to internal workCh.
// Send blocks until workFn is received.
func (p *WorkerPool) Send(workFn WorkFn) {
	p.workCh <- workFn
}

// Add adds delta number of workers to this pool.
// This will create delta number of goroutines.
func (p *WorkerPool) Add(delta uint32) (newAliveLen int) {
	p.mu.Lock()
	for i := uint32(0); i < delta; i++ {
		workerId := p.workerIdx
		p.workerIdx++
		worker := p.workerConstructor(
			workerId,
			func() { atomic.AddInt64(&p.activeWorkerNum, 1) },
			func() { atomic.AddInt64(&p.activeWorkerNum, -1) },
		)
		// callWorkerStart calls wg.Done() before it exits.
		p.wg.Add(1)
		go p.callWorkerStart(worker, true, p.abnormalReturnCb)

		p.workers[worker.Id()] = worker
	}
	p.mu.Unlock()
	alive, _ := p.Len()
	return alive
}

var (
	errGoexit = errors.New("runtime.Goexit was called")
)

type panicErr struct {
	err   interface{}
	stack []byte
}

// Error implements error interface.
func (p *panicErr) Error() string {
	return fmt.Sprintf("%v\n\n%s", p.err, p.stack)
}

func (p *WorkerPool) callWorkerStart(worker *Worker[int], shouldRecover bool, abnormalReturnCb func(error)) (workerErr error) {
	var normalReturn, recovered bool
	var abnormalReturnErr error
	// see https://cs.opensource.google/go/x/sync/+/0de741cf:singleflight/singleflight.go;l=138-200;drc=0de741cfad7ff3874b219dfbc1b9195b58c7c490
	defer func() {
		// Done will be done right before the exit.
		defer p.wg.Done()
		p.mu.Lock()
		delete(p.workers, worker.Id())
		delete(p.sleepingWorkers, worker.Id())
		p.mu.Unlock()

		if !normalReturn && !recovered {
			abnormalReturnErr = errGoexit
		}
		if !normalReturn {
			abnormalReturnCb(abnormalReturnErr)
		}
		if recovered && !shouldRecover {
			panic(abnormalReturnErr)
		}
	}()

	func() {
		defer func() {
			if err := recover(); err != nil {
				abnormalReturnErr = &panicErr{
					err:   err,
					stack: debug.Stack(),
				}
			}
		}()
		workerErr = worker.Start()
		normalReturn = true
	}()
	if !normalReturn {
		recovered = true
	}
	return
}

// Remove removes delta number of randomly selected workers from this pool.
// Removed workers could be held as sleeping if they are still working on workFn.
func (p *WorkerPool) Remove(delta uint32) (alive int, sleeping int) {
	p.mu.Lock()
	var count uint32
	for _, worker := range p.workers {
		if count < delta {
			worker.Stop()
			delete(p.workers, worker.Id())
			p.sleepingWorkers[worker.Id()] = worker
			count++
		} else {
			break
		}
	}
	p.mu.Unlock()
	return p.Len()
}

// Len returns number of workers.
// alive is active worker. sleeping is worker removed by Remove while still working on its job.
func (p *WorkerPool) Len() (alive int, sleeping int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.workers), len(p.sleepingWorkers)
}

// ActiveWorkerNum returns number of actively working worker.
func (p *WorkerPool) ActiveWorkerNum() int64 {
	return atomic.LoadInt64(&p.activeWorkerNum)
}

// Kill kills all workers.
func (p *WorkerPool) Kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.workers {
		w.Kill()
	}
}

// Wait waits for all workers to stop.
// Calling this without sleeping or removing all worker may block forever.
func (p *WorkerPool) Wait() {
	p.wg.Wait()
}
