package hub

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

const (
	// chans, goroutines, workers timeout
	timeout = 100 * time.Millisecond
)

// wgWaitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
func wgWaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			<-timer.C
		}
	}()

	select {
	case <-c:
		return false // completed normally
	case <-timer.C:
		return true // timed out
	}
}

// use:
//    (github.com/stretchr/testify/mock)
// or (https://github.com/golang/mock)
// or (https://github.com/vektra/mockery (no))
type mockMsg struct {
	unrefHit int
	refDelta int64
}

func (mm *mockMsg) Unref() {
	mm.unrefHit += 1
}

func (mm *mockMsg) RefDelta(delta int64) {
	mm.refDelta += delta
}

func TestHub(t *testing.T) {
	// workers to subscribe
	subsCount := 1000

	tests := []struct {
		subCapacity int

		// msg to publish
		msg interface{}
	}{
		{-2, time.Now().UnixNano()},
		{0, time.Now().UnixNano()},
		{1, time.Now().UnixNano()},
		{10, time.Now()},
		{100, time.Now().UnixNano()},

		{-1, &mockMsg{}},
		{5, &mockMsg{}},

		{3, &mockMsg{}},
	}

	for _, tt := range tests {
		func() {
			hub := NewHub()

			if hub == nil {
				t.Fatalf("hub allocation failed")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go hub.Start(ctx)

			wgStart := sync.WaitGroup{}
			wgStart.Add(subsCount)

			wg := sync.WaitGroup{}
			wg.Add(subsCount)

			for i := 0; i < subsCount; i++ {
				go func(i int) {
					defer wg.Done()

					sub := hub.Sub(tt.subCapacity)
					defer func() {
						hub.UnsubWithTimeout(sub, timeout)
					}()

					wgStart.Done()

					msg, ok := <-sub

					if !ok {
						t.Error("hub subscription unexpectedly closed")
					}

					if msg != tt.msg {
						t.Error("got wrong msg from subscripton")
					}
				}(i)
			}

			if wgWaitTimeout(&wgStart, timeout) {
				t.Fatalf("workers start failed")
			}

			hub.Pub(tt.msg)

			if wgWaitTimeout(&wg, timeout) {
				t.Fatalf("not all workers are done in time")
			}

			hub.Stop()

			if msg, ok := tt.msg.(*mockMsg); ok {
				if msg.refDelta != int64(subsCount) {
					t.Error("ref-delta wasn't properly called on msg")
				}
			}

			doneCtx, cancelDoneCtx := context.WithTimeout(ctx, timeout)
			defer cancelDoneCtx()
			hub.DoneWithContext(doneCtx)

			if err := doneCtx.Err(); err != nil && errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("hub doesn't done in time")
			}
		}()
	}
}

func TestStopDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub()
	go hub.Start(ctx)

	_ = hub.Sub(-1)
	_ = hub.Sub(0)
	_ = hub.Sub(1)

	hub.StopNonBlock()
	hub.StopNonBlock()

	stopCtx, cancelStopCtx := context.WithTimeout(ctx, timeout)
	defer cancelStopCtx()
	hub.StopWithContext(stopCtx)

	if err := stopCtx.Err(); err == nil {
		t.Errorf("hub stop doesn't care about context")
	}

	doneWg := sync.WaitGroup{}
	doneWg.Add(1)
	go func() {
		defer doneWg.Done()
		hub.Done()
	}()

	if wgWaitTimeout(&doneWg, timeout) {
		t.Fatalf("hub done failed")
	}

	doneCtx, cancelDoneCtx := context.WithTimeout(ctx, timeout)
	defer cancelDoneCtx()
	hub.DoneWithContext(doneCtx)

	if err := doneCtx.Err(); err == nil {
		t.Errorf("hub done doesn't care about context")
	}
}

func TestStartNilCtx(t *testing.T) {
	hub := NewHub()
	go hub.Start(nil)

	hub.StopNonBlock()
	hub.Done()
}

// test hub can be canceled via ctx
func TestStartWithCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	hub := NewHub()
	go hub.Start(ctx)

	// cancel hub via ctx
	cancel()

	doneWg := sync.WaitGroup{}
	doneWg.Add(1)
	go func() {
		defer doneWg.Done()
		hub.Done()
	}()

	if wgWaitTimeout(&doneWg, timeout) {
		t.Fatalf("hub done failed")
	}
}
