package hub

import (
	"context"
	"time"
)

type Hub struct {
	pub   chan interface{}
	sub   chan chan interface{}
	unsub chan chan interface{}

	done chan struct{}
	stop chan struct{}
}

func msgUnref(any interface{}) {
	if u, ok := any.(interface{ Unref() }); ok {
		u.Unref()
	}
}

func msgRefDelta(any interface{}, delta int64) {
	if rd, ok := any.(interface{ RefDelta(int64) }); ok {
		rd.RefDelta(delta)
	}
}

func (h *Hub) Stop() { h.stop <- struct{}{} }
func (h *Hub) StopNonBlock() {
	select {
	case h.stop <- struct{}{}:
	default:
	}
}

func (h *Hub) StopWithContext(ctx context.Context) {
	select {
	case h.stop <- struct{}{}:
	case <-ctx.Done():
	}
}
func (h *Hub) Done() { <-h.done }
func (h *Hub) DoneWithContext(ctx context.Context) {
	select {
	case <-h.done:
	case <-ctx.Done():
	}
}

func (h *Hub) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.TODO()
	}

	defer func() {
		select {
		case h.done <- struct{}{}:
		default:
		}
	}()

	subs := map[chan interface{}]struct{}{}

	fnBroadcast := func(msg interface{}) {
		if len(subs) == 0 {
			msgUnref(msg)
		} else {
			msgRefDelta(msg, int64(len(subs)))

			for sub := range subs {
				select {
				case sub <- msg:
				default:
					//! TODO: drop and requeu
					msgUnref(msg)
				}
			}
		}
	}

	fnCloseSub := func(sub chan interface{}) {
		for { // drain
			select {
			case msg := <-sub:
				msgUnref(msg)
			default:
				close(sub)
				return
			}
		}
	}

	fnClose := func() {
		for sub := range subs {
			fnCloseSub(sub)
		}
	}

	for {
		select {
		case <-h.stop:
			fnClose()
			return

		case <-ctx.Done():
			fnClose()
			return

		case sub := <-h.sub:
			subs[sub] = struct{}{}

		case sub := <-h.unsub:
			delete(subs, sub)
			fnCloseSub(sub)

		case msg := <-h.pub:
			fnBroadcast(msg)
		}
	}
}

func (h *Hub) Sub(capacity int) (sub chan interface{}) {
	if capacity <= 0 {
		sub = make(chan interface{})
	} else {
		sub = make(chan interface{}, capacity)
	}
	h.sub <- sub

	return sub
}

func (h *Hub) SubWithContext(ctx context.Context, capacity int) (sub chan interface{}) {
	if capacity <= 0 {
		sub = make(chan interface{})
	} else {
		sub = make(chan interface{}, capacity)
	}

	select {
	case h.sub <- sub:
	case <-ctx.Done():
	}

	return sub
}

func (h *Hub) Unsub(sub chan interface{}) {
	h.unsub <- sub
}

func (h *Hub) UnsubNonBlock(sub chan interface{}) {
	select {
	case h.unsub <- sub:
	default:
	}
}

func (h *Hub) UnsubWithTimeout(sub chan interface{}, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			<-timer.C
		}
	}()

	select {
	case h.unsub <- sub:
	case <-timer.C:
	}
}

func (h *Hub) UnsubWithContext(ctx context.Context, sub chan interface{}) {
	select {
	case h.unsub <- sub:
	case <-ctx.Done():
	}
}

func (h *Hub) Pub(msg interface{}) {
	h.pub <- msg
}

func (h *Hub) PubWithContext(ctx context.Context, msg interface{}) {
	select {
	case h.pub <- msg:
	case <-ctx.Done():
	}
}

func (h *Hub) Init() {
	h.pub = make(chan interface{})
	h.sub = make(chan chan interface{})
	h.unsub = make(chan chan interface{})

	h.stop = make(chan struct{}, 1)
	h.done = make(chan struct{}, 1)
}

func NewHub() *Hub {
	h := new(Hub)
	h.Init()

	return h
}
