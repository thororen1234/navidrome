package downloader

import (
	"context"
	"sync"

	"github.com/navidrome/navidrome/utils/singleton"
)

// registry tracks cancel functions for jobs the Worker currently has running, so Service.Cancel
// can stop a job that's already downloading - not just one still sitting in the queue. Shared
// between Service and Worker (both hold the same *registry - see GetRegistry), it is the only
// coupling between the two: Service never touches the DB for a running job's terminal status,
// leaving that to the Worker itself once it observes the cancellation.
//
// It also carries the worker's wake-up signal, so a newly submitted job starts right away
// instead of waiting for the next poll tick.
type registry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	// pending holds cancel requests for jobs the worker has claimed but not registered yet (the
	// short window between ClaimNext and register); register honors them immediately.
	pending map[string]struct{}
	wake    chan struct{}
}

// GetRegistry returns the process-wide registry, which is what wire injects. It must be one
// instance: wire builds every injector's graph separately, so with a plain constructor the
// worker and each API router got their own registry - cancels never reached the running job
// and submissions never woke the worker.
func GetRegistry() *registry {
	return singleton.GetInstance(NewRegistry)
}

// NewRegistry returns a fresh, unshared registry (for tests).
func NewRegistry() *registry {
	return &registry{
		cancels: map[string]context.CancelFunc{},
		pending: map[string]struct{}{},
		wake:    make(chan struct{}, 1),
	}
}

func (r *registry) register(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[id]; ok {
		delete(r.pending, id)
		cancel()
	}
	r.cancels[id] = cancel
}

func (r *registry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
	delete(r.pending, id)
}

// cancel invokes the job's cancel function if it is still registered (i.e. still running),
// reporting whether it found one. A job that finished between the caller's status check and
// this call simply won't be found here - harmless, since contexts are safe to cancel multiple
// times or after completion.
func (r *registry) cancel(id string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// cancelClaimed cancels a job the DB says is downloading: right away if it's registered, or as
// soon as the worker registers it.
func (r *registry) cancelClaimed(id string) {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	if !ok {
		r.pending[id] = struct{}{}
	}
	r.mu.Unlock()
	if ok {
		cancel()
	}
}

// notify wakes the worker without blocking; one pending signal is enough to make it re-check
// the queue.
func (r *registry) notify() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
