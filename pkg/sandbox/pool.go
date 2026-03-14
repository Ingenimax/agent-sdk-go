package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Container represents a running sandbox container.
type Container struct {
	ID        string
	Name      string
	Ready     bool
	CreatedAt time.Time
}

// Pool manages a set of warm sandbox containers with round-robin selection.
type Pool struct {
	containers []Container
	mu         sync.Mutex
	nextIdx    int
	closeFn    func(ctx context.Context, id string) error
}

// NewPool creates a pool with pre-created containers.
func NewPool(containers []Container, closeFn func(ctx context.Context, id string) error) *Pool {
	return &Pool{
		containers: containers,
		closeFn:    closeFn,
	}
}

// Acquire returns the next available container using round-robin selection.
func (p *Pool) Acquire(ctx context.Context) (*Container, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.containers) == 0 {
		return nil, ErrContainerUnhealthy
	}

	c := &p.containers[p.nextIdx]
	p.nextIdx = (p.nextIdx + 1) % len(p.containers)

	if !c.Ready {
		return nil, fmt.Errorf("%w: container %s is not ready", ErrContainerUnhealthy, c.Name)
	}

	return c, nil
}

// Close stops and removes all containers in the pool.
func (p *Pool) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, c := range p.containers {
		if p.closeFn != nil {
			if err := p.closeFn(ctx, c.ID); err != nil {
				lastErr = err
			}
		}
	}
	p.containers = nil
	return lastErr
}

// MarkUnhealthy marks a container as not ready.
func (p *Pool) MarkUnhealthy(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.containers {
		if p.containers[i].ID == id {
			p.containers[i].Ready = false
			break
		}
	}
}
