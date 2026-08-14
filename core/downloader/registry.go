package downloader

import (
	"context"
	"sync"
)

// registry tracks cancel functions for jobs the Worker currently has running, so Service.Cancel
// can stop a job that's already downloading - not just one still sitting in the queue. Shared
// between Service and Worker (both hold the same *registry, injected via wire), it is the only
// coupling between the two: Service never touches the DB for a running job's terminal status,
// leaving that to the Worker itself once it observes the cancellation.
type registry struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewRegistry() *registry {
	return &registry{cancels: map[string]context.CancelFunc{}}
}

func (r *registry) register(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[id] = cancel
}

func (r *registry) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
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
