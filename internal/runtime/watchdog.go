package runtime

import (
	"io"
	"time"
)

// idleWatchdog fires onExpire if no Reset() call happens within timeout.
// It is used by [withIdleWatchdog] to time out idle upstream reads.
type idleWatchdog struct {
	reset    chan struct{}
	stop     chan struct{}
	expired  chan struct{}
	timeout  time.Duration
	onExpire func()
}

func newIdleWatchdog(timeout time.Duration, onExpire func()) *idleWatchdog {
	w := &idleWatchdog{
		reset:    make(chan struct{}, 1),
		stop:     make(chan struct{}),
		expired:  make(chan struct{}),
		timeout:  timeout,
		onExpire: onExpire,
	}
	go w.run()
	return w
}

func (w *idleWatchdog) run() {
	timer := time.NewTimer(w.timeout)
	armed := true
	for {
		select {
		case <-w.stop:
			timer.Stop()
			return
		case <-w.reset:
			if armed {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			timer.Reset(w.timeout)
			armed = true
		case <-timer.C:
			select {
			case <-w.expired:
			default:
				close(w.expired)
			}
			if w.onExpire != nil {
				w.onExpire()
			}
			return
		}
	}
}

// Reset pushes a reset signal into the watchdog's loop. The channel is
// buffered to size 1 so a Reset that races with the loop is a no-op
// rather than blocking the caller.
func (w *idleWatchdog) Reset() {
	select {
	case w.reset <- struct{}{}:
	default:
	}
}

// Stop terminates the watchdog. Calling Stop more than once is a no-op.
func (w *idleWatchdog) Stop() {
	select {
	case <-w.stop:
		return
	default:
		close(w.stop)
	}
}

// watchdogReader wraps an io.Reader and resets the watchdog on every
// successful read.
type watchdogReader struct {
	r io.Reader
	w *idleWatchdog
}

func (w *watchdogReader) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if n > 0 {
		w.w.Reset()
	}
	return n, err
}

// withIdleWatchdog returns an [io.ReadCloser] that fires onTimeout
// when no bytes have been read for the given duration. The returned
// closer stops the watchdog when closed.
func withIdleWatchdog(r io.ReadCloser, timeout time.Duration, onTimeout func()) io.ReadCloser {
	wd := newIdleWatchdog(timeout, onTimeout)
	return &watchdogCloser{ReadCloser: r, r: &watchdogReader{r: r, w: wd}}
}

type watchdogCloser struct {
	io.ReadCloser
	r *watchdogReader
}

func (w *watchdogCloser) Read(p []byte) (int, error) {
	return w.r.Read(p)
}

func (w *watchdogCloser) Close() error {
	w.r.w.Stop()
	return w.ReadCloser.Close()
}
