package hub

import (
	"context"
	"sync"
	"testing"
	"time"
)

// waitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

func TestHubUnbuffered(t *testing.T) {
	// msg to publish
	msg := time.Now().UnixNano()
	// workers to subscribe
	subsCount := 1000

	hub := new(Hub)

	if hub == nil {
		t.Fatalf("hub allocation failed")
	}

	hub.Init()

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

			sub := hub.Sub(0)

			wgStart.Done()

			msgActual, ok := <-sub

			if !ok {
				t.Error("hub subscription unexpectedly closed")
			}

			if msg != msgActual {
				t.Error("got wrong msg from subscripton")
			}
		}(i)
	}

	if waitTimeout(&wgStart, 100*time.Millisecond) {
		t.Fatalf("workers start failed")
	}

	hub.Pub(msg)

	if waitTimeout(&wg, 100*time.Millisecond) {
		t.Fatalf("not all workers are done in time")
	}
}
