package gopherpool

import (
	"context"
	"fmt"

	"github.com/ngicks/gommon/state"
	atomicparam "github.com/ngicks/type-param-common/sync-param/atomic-param"
)

type WorkFn = func(ctx context.Context)

// Worker represents a single task executor.
// It will work on a single task at a time.
// It may be in stopped-state where loop is stopped,
// working-state where looping in goroutine,
// or ended-state where no way is given to step into working-state again.
type Worker[T any] struct {
	*state.WorkingStateChecker
	workingInner *state.WorkingStateSetter
	*state.EndedStateChecker
	endedInner *state.EndedStateSetter

	id             T
	stopCh         chan struct{}
	killCh         chan struct{}
	workCh         <-chan WorkFn
	onWorkReceived func()
	onWorkDone     func()
	cancel         atomicparam.Value[*context.CancelFunc]
}

func NewWorker[T any](id T, workCh <-chan WorkFn, onTaskReceived, onWorkDone func()) (*Worker[T], error) {
	if workCh == nil {
		return nil, fmt.Errorf("workCh is nil")
	}

	if onTaskReceived == nil {
		onTaskReceived = func() {}
	}
	if onWorkDone == nil {
		onWorkDone = func() {}
	}

	workingStateChecker, workingStateInner := state.NewWorkingState()
	endedStateChecker, endedStateInner := state.NewEndedState()

	worker := &Worker[T]{
		WorkingStateChecker: workingStateChecker,
		workingInner:        workingStateInner,
		EndedStateChecker:   endedStateChecker,
		endedInner:          endedStateInner,
		id:                  id,
		stopCh:              make(chan struct{}, 1),
		killCh:              make(chan struct{}),
		workCh:              workCh,
		onWorkReceived:      onTaskReceived,
		onWorkDone:          onWorkDone,
		cancel:              atomicparam.NewValue[*context.CancelFunc](),
	}
	return worker, nil
}

// Start starts worker loop. It blocks until Stop and/or Kill is called,
// or conditions below are met.
// w will be ended if workCh is closed or workFn returns abnormally.
//
//   - Start returns `ErrAlreadyEnded` if worker is already ended.
//   - Start returns `ErrAlreadyStarted` if worker is already started.
func (w *Worker[T]) Start() (err error) {
	if w.IsEnded() {
		return ErrAlreadyEnded
	}
	if !w.workingInner.SetWorking() {
		return ErrAlreadyStarted
	}
	defer w.workingInner.SetWorking(false)

	defer func() {
		select {
		case <-w.stopCh:
		default:
		}
	}()

	var normalReturn bool
	defer func() {
		if !normalReturn {
			w.endedInner.SetEnded()
		}
	}()

loop:
	for {
		select {
		case <-w.killCh:
			// in case of racy kill
			break loop
		case <-w.stopCh:
			break loop
		default:
			select {
			case <-w.stopCh:
				break loop
			case workFn, ok := <-w.workCh:
				if !ok {
					w.endedInner.SetEnded()
					break loop
				}
				func() {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()

					w.cancel.Store(&cancel)
					// Prevent it from being held after when it is unneeded.
					defer w.cancel.Store(nil)

					select {
					// in case of racy kill
					case <-w.killCh:
						return
					default:
					}

					w.onWorkReceived()
					defer w.onWorkDone()
					workFn(ctx)
				}()
			}
		}
	}
	// If task exits abnormally, called runtime.Goexit or panicking, it would not reach this line.
	normalReturn = true
	return
}

// Stop stops an active Start loop.
// Stop does not cancel contex passed to workFn, just waits until it returns.
//
// Stop will stops next Start immediately if Start is not doing its loop when Stop is called.
func (w *Worker[T]) Stop() {
	select {
	case <-w.stopCh:
	default:
	}
	w.stopCh <- struct{}{}
	return
}

// Kill kills this worker.
// If a task is being worked at the time of invocation,
// a contex passed to the task will be cancelled immediately.
// Kill makes this worker to step into ended state, making it impossible to Start-ed again.
func (w *Worker[T]) Kill() {
	if w.endedInner.SetEnded() {
		close(w.killCh)
	}

	if cancel := w.cancel.Load(); cancel != nil {
		(*cancel)()
	}

	w.Stop()
}

// Id is getter of id.
func (w *Worker[T]) Id() T {
	return w.id
}
