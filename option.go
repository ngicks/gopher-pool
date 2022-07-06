package gopherpool

import "log"

// Option is an option that changes WoerkerPool instance.
// This can be used in NewWorkerPool.
type Option = func(w *WorkerPool) *WorkerPool

// SetDefaultAbnormalReturnCb is an Option that
// overrides abnormal-return cb with cb.
//
// cb is called if and only if WorkFn is returned abnormally.
// cb may be called multiple time simultaneously.
func SetAbnormalReturnCb(cb func(err error)) Option {
	return func(w *WorkerPool) *WorkerPool {
		w.abnormalReturnCb = cb
		return w
	}
}

// SetDefaultAbnormalReturnCb is an Option that,
//
//   - overrides abnormal-return cb.
//   - simply log.Println runtime-panic or runtime.Goexit-is-called error.
//
// cb is called if and only if WorkFn is returned abnormally.
// cb may be called multiple time simultaneously.
func SetDefaultAbnormalReturnCb() Option {
	return func(w *WorkerPool) *WorkerPool {
		w.abnormalReturnCb = func(err error) { log.Println(err) }
		return w
	}
}

// SetWorkerConstructor sets workerConstructor and assosiated workCh.
// workFn must be sent throught this workCh.
func SetWorkerConstructor(workCh chan WorkFn, workerConstructor WorkerConstructor) Option {
	return func(w *WorkerPool) *WorkerPool {
		w.workCh = workCh
		w.workerConstructor = workerConstructor
		return w
	}
}

// SetWorkerConstructor sets default worker construtor implementation built from given args.
// workFn must be sent throught this workCh.
// Both of onTaskReceived and onTaskDone can be nil.
func SetDefaultWorkerConstructor(workCh chan WorkFn, onTaskReceived func(), onTaskDone func()) Option {
	return func(w *WorkerPool) *WorkerPool {
		w.workCh = workCh
		w.workerConstructor = BuildWorkerConstructor(workCh, onTaskReceived, onTaskDone)
		return w
	}
}
