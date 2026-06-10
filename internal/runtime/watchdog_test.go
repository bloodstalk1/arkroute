package runtime

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type slowReader struct {
	data  []byte
	pos   int
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *slowReader) Close() error { return nil }

func TestIdleWatchdogFiresOnIdle(t *testing.T) {
	expired := make(chan struct{})
	timeout := 50 * time.Millisecond
	r := &slowReader{data: []byte("a"), delay: 5 * time.Millisecond}
	wrapped := withIdleWatchdog(r, timeout, func() {
		select {
		case <-expired:
		default:
			close(expired)
		}
	})
	defer wrapped.Close()

	buf := make([]byte, 1)
	readStart := time.Now()
	if _, err := wrapped.Read(buf); err != nil {
		t.Fatalf("first Read: %v", err)
	}
	t.Logf("Read returned at %v", time.Since(readStart))
	select {
	case <-expired:
		t.Fatal("watchdog fired before idle timeout elapsed")
	case <-time.After(timeout / 2):
		t.Logf("checked not-fired at %v", time.Since(readStart))
	}
	select {
	case <-expired:
		t.Logf("fired at %v", time.Since(readStart))
	case <-time.After(timeout * 4):
		t.Fatalf("watchdog did not fire within expected window; total elapsed %v", time.Since(readStart))
	}
}

func TestIdleWatchdogResetsOnRead(t *testing.T) {
	expired := make(chan struct{})
	timeout := 100 * time.Millisecond
	r := &slowReader{data: []byte("abcdef"), delay: 10 * time.Millisecond}
	wrapped := withIdleWatchdog(r, timeout, func() {
		select {
		case <-expired:
		default:
			close(expired)
		}
	})
	defer wrapped.Close()

	buf := make([]byte, 1)
	for i := 0; i < 5; i++ {
		if _, err := wrapped.Read(buf); err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("Read %d: %v", i, err)
			}
		}
		select {
		case <-expired:
			t.Fatalf("watchdog fired at read %d despite continuous reads", i)
		default:
		}
	}
	select {
	case <-expired:
		t.Fatal("watchdog fired while reads were continuous")
	case <-time.After(timeout / 2):
	}
}

func TestIdleWatchdogStopPreventsFire(t *testing.T) {
	fired := false
	timeout := 50 * time.Millisecond
	r := &slowReader{data: []byte("x"), delay: 10 * time.Millisecond}
	wrapped := withIdleWatchdog(r, timeout, func() { fired = true })
	if _, err := wrapped.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	if err := wrapped.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(timeout * 3)
	if fired {
		t.Fatal("watchdog fired after Close")
	}
}

func TestIdleWatchdogOnExpireReceivesError(t *testing.T) {
	r := &slowReader{data: []byte(strings.Repeat("a", 1024)), delay: 80 * time.Millisecond}
	timeout := 30 * time.Millisecond
	done := make(chan struct{})
	wrapped := withIdleWatchdog(r, timeout, func() { close(done) })
	defer wrapped.Close()
	buf := make([]byte, 16)
	_, _ = wrapped.Read(buf)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("onExpire did not fire")
	}
}

func TestIdleWatchdogConcurrentResetStop(t *testing.T) {
	for i := 0; i < 50; i++ {
		r := &slowReader{data: []byte("a"), delay: time.Millisecond}
		wrapped := withIdleWatchdog(r, 20*time.Millisecond, func() {})
		go func() {
			for j := 0; j < 20; j++ {
				wrapped.Read(make([]byte, 1))
			}
		}()
		time.Sleep(5 * time.Millisecond)
		wrapped.Close()
	}
}
