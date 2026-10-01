package signal

import (
	"context"
	"sync"
	"time"
)

type ActivityUpdater interface{ Update() }

type ActivityTimer struct {
	sync.Mutex
	updated     chan struct{}
	timer       *time.Timer
	onTimeout   func()
	stopped     bool
	generation  uint64
	stopContext func() bool
}

func (t *ActivityTimer) Update() {
	select {
	case t.updated <- struct{}{}:
	default:
	}
}

func (t *ActivityTimer) check(gen uint64, interval time.Duration) {
	t.Lock()
	if t.stopped || gen != t.generation {
		t.Unlock()
		return
	}
	select {
	case <-t.updated:
		t.timer = time.AfterFunc(interval, func() { t.check(gen, interval) })
		t.Unlock()
	default:
		callback := t.closeLocked()
		t.Unlock()
		if callback != nil {
			callback()
		}
	}
}

// closeLocked detaches the callback; callers invoke it only after unlocking.
func (t *ActivityTimer) closeLocked() func() {
	t.stopped = true
	t.generation++
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if t.stopContext != nil {
		t.stopContext()
		t.stopContext = nil
	}
	callback := t.onTimeout
	t.onTimeout = nil
	return callback
}

// Close stops the timer without invoking its timeout callback.
func (t *ActivityTimer) Close() error {
	t.Lock()
	t.closeLocked()
	t.Unlock()
	return nil
}

func (t *ActivityTimer) SetTimeout(timeout time.Duration) {
	t.Lock()
	if t.stopped {
		t.Unlock()
		return
	}
	if timeout <= 0 {
		callback := t.closeLocked()
		t.Unlock()
		if callback != nil {
			callback()
		}
		return
	}
	if t.timer != nil {
		t.timer.Stop()
	}
	t.generation++
	gen := t.generation
	// A new interval starts now; activity in the old interval is not carried over.
	select {
	case <-t.updated:
	default:
	}
	t.timer = time.AfterFunc(timeout, func() { t.check(gen, timeout) })
	t.Unlock()
}

func CancelAfterInactivity(ctx context.Context, cancel context.CancelFunc, timeout time.Duration) *ActivityTimer {
	t := &ActivityTimer{updated: make(chan struct{}, 1), onTimeout: cancel}
	t.Lock()
	t.stopContext = context.AfterFunc(ctx, func() { _ = t.Close() })
	t.Unlock()
	t.SetTimeout(timeout)
	return t
}
