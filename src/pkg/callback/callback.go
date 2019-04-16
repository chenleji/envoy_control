package callback

import (
	"context"
	"github.com/astaxie/beego/logs"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"sync"
)

type Callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mu       sync.Mutex
}

func NewCallbacks(ch chan struct{}) *Callbacks {
	return &Callbacks{
		signal:   ch,
		fetches:  0,
		requests: 0,
	}
}

func (cb *Callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	logs.Debug("cb.Report()  Callbacks. fetches:", cb.fetches, "requests:", cb.requests)
}

func (cb *Callbacks) OnStreamOpen(ctx context.Context, id int64, typ string) error {
	logs.Debug("OnStreamOpen:", id, "open for ", typ)
	return nil
}

func (cb *Callbacks) OnStreamClosed(id int64) {
	logs.Debug("OnStreamClosed:", id, "closed")
}

func (cb *Callbacks) OnStreamRequest(int64, *v2.DiscoveryRequest) error {
	logs.Debug("OnStreamRequest")
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.requests++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}

	return nil
}

func (cb *Callbacks) OnStreamResponse(int64, *v2.DiscoveryRequest, *v2.DiscoveryResponse) {
	logs.Debug("OnStreamResponse...")
	cb.Report()
}

func (cb *Callbacks) OnFetchRequest(ctx context.Context, req *v2.DiscoveryRequest) error {
	logs.Debug("OnFetchRequest...")
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.fetches++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}

	return nil
}

func (cb *Callbacks) OnFetchResponse(*v2.DiscoveryRequest, *v2.DiscoveryResponse) {}
