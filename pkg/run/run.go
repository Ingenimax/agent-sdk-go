// Package run gives an agent run an identity, a registry and a cancel switch.
//
// Agent.Run is synchronous and anonymous: it returns a string and there is no
// handle on the work while it is in flight. Nothing can ask what is running,
// attach to it, or stop one specific run. The only per-run object in the SDK is
// an unexported usage tracker living in a context value, which dies with the
// stack frame.
//
// This package adds the missing layer, and deliberately nothing more:
//
//   - a RunID minted per run;
//   - a Handle to await the result, observe status, or cancel;
//   - a Manager that can list live runs and cancel one by ID.
//
// # Relationship to pkg/task
//
// pkg/task is a task *lifecycle* service: create a task, plan it, have a human
// approve the plan, log against it. It concerns work a person tracks. This
// package concerns a single agent invocation that is in flight right now. They
// are not alternatives, and neither is built on the other.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ErrNotFound is returned when no run matches an ID.
var ErrNotFound = errors.New("run not found")

// Status is the lifecycle state of a run.
type Status string

const (
	// StatusRunning means the run is in flight.
	StatusRunning Status = "running"
	// StatusSucceeded means the run produced a result.
	StatusSucceeded Status = "succeeded"
	// StatusFailed means the run returned an error.
	StatusFailed Status = "failed"
	// StatusCanceled means the run was cancelled before completing.
	StatusCanceled Status = "canceled"
)

// Terminal reports whether a status is final.
func (s Status) Terminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCanceled
}

// Agent is the subset of an agent this package needs.
//
// Declared here rather than imported so pkg/run does not depend on pkg/agent,
// which keeps the dependency pointing one way and lets callers wrap or fake an
// agent freely.
type Agent interface {
	Run(ctx context.Context, input string) (string, error)
	GetName() string
}

// StreamingAgent is implemented by agents that can stream.
type StreamingAgent interface {
	Agent
	RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error)
}

// Info is a point-in-time snapshot of a run.
type Info struct {
	ID        string
	AgentName string
	Input     string
	Status    Status
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
}

// Duration returns how long the run took, or how long it has been running.
func (i Info) Duration() time.Duration {
	if i.EndedAt.IsZero() {
		return time.Since(i.StartedAt)
	}
	return i.EndedAt.Sub(i.StartedAt)
}

// Handle is a live reference to one run.
type Handle struct {
	id        string
	agentName string
	input     string

	cancel context.CancelFunc
	done   chan struct{}

	mu        sync.Mutex
	status    Status
	result    string
	err       error
	startedAt time.Time
	endedAt   time.Time

	events <-chan interfaces.AgentStreamEvent
}

// ID returns the run's identifier.
func (h *Handle) ID() string { return h.id }

// Info returns a snapshot of the run's current state.
func (h *Handle) Info() Info {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Info{
		ID:        h.id,
		AgentName: h.agentName,
		Input:     h.input,
		Status:    h.status,
		StartedAt: h.startedAt,
		EndedAt:   h.endedAt,
		Err:       h.err,
	}
}

// Done returns a channel closed when the run reaches a terminal state.
func (h *Handle) Done() <-chan struct{} { return h.done }

// Events returns the run's stream, or nil if it was not started with Stream.
//
// The channel is closed when the run finishes.
func (h *Handle) Events() <-chan interfaces.AgentStreamEvent { return h.events }

// Wait blocks until the run finishes and returns its result.
//
// Waiting respects the caller's own context: a caller giving up does not cancel
// the run, which continues in the background. Use Cancel to stop it.
func (h *Handle) Wait(ctx context.Context) (string, error) {
	select {
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.result, h.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Cancel stops the run. It is safe to call more than once, and on a run that
// has already finished.
func (h *Handle) Cancel() {
	h.cancel()
}

func (h *Handle) finish(status Status, result string, err error) {
	h.mu.Lock()
	if h.status.Terminal() {
		h.mu.Unlock()
		return
	}
	h.status = status
	h.result = result
	h.err = err
	h.endedAt = time.Now()
	h.mu.Unlock()

	close(h.done)
}

// Manager starts runs and keeps track of the ones in flight.
//
// The zero value is not usable; call NewManager.
type Manager struct {
	mu   sync.RWMutex
	runs map[string]*Handle

	// retain bounds how many finished runs stay listable. Without a bound a
	// long-lived service accumulates every run it has ever executed.
	retain   int
	finished []string
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithRetainedRuns sets how many finished runs remain listable. Defaults to 100.
//
// A bound matters: a registry that keeps every run forever is a memory leak in
// any service that stays up.
func WithRetainedRuns(n int) ManagerOption {
	return func(m *Manager) {
		if n >= 0 {
			m.retain = n
		}
	}
}

// NewManager creates a run manager.
func NewManager(options ...ManagerOption) *Manager {
	m := &Manager{
		runs:   make(map[string]*Handle),
		retain: 100,
	}
	for _, option := range options {
		option(m)
	}
	return m
}

// StartOption configures a single run.
type StartOption func(*startConfig)

type startConfig struct {
	timeout  time.Duration
	detached bool
}

// WithTimeout bounds the run's duration.
func WithTimeout(d time.Duration) StartOption {
	return func(c *startConfig) { c.timeout = d }
}

// WithDetached runs independently of the caller's context, so the run survives
// the request that started it.
//
// Without this a background run started inside an HTTP handler dies when the
// client disconnects, which is rarely what "background" is meant to mean. With
// it, the only ways to stop the run are Cancel or a timeout -- so a timeout is
// strongly advised alongside it.
func WithDetached() StartOption {
	return func(c *startConfig) { c.detached = true }
}

// Start runs an agent in the background and returns immediately.
func (m *Manager) Start(ctx context.Context, agent Agent, input string, options ...StartOption) *Handle {
	h, runCtx := m.begin(ctx, agent, input, options...)

	go func() {
		result, err := agent.Run(runCtx, input)
		m.complete(h, runCtx, result, err)
	}()

	return h
}

// Stream runs an agent in the background and exposes its events.
//
// The agent must implement StreamingAgent; otherwise the returned handle
// finishes immediately with an error.
func (m *Manager) Stream(ctx context.Context, agent Agent, input string, options ...StartOption) *Handle {
	h, runCtx := m.begin(ctx, agent, input, options...)

	streamer, ok := agent.(StreamingAgent)
	if !ok {
		h.finish(StatusFailed, "", fmt.Errorf("agent %q does not support streaming", agent.GetName()))
		m.retire(h.id)
		return h
	}

	source, err := streamer.RunStream(runCtx, input)
	if err != nil {
		h.finish(StatusFailed, "", err)
		m.retire(h.id)
		return h
	}

	// Re-broadcast so the handle owns a channel it can close on completion,
	// and so content can be accumulated into the run's result.
	out := make(chan interfaces.AgentStreamEvent, 32)
	h.events = out

	go func() {
		defer close(out)

		var content string
		var streamErr error

		for event := range source {
			if event.Type == interfaces.AgentEventContent {
				content += event.Content
			}
			if event.Error != nil {
				streamErr = event.Error
			}
			select {
			case out <- event:
			case <-runCtx.Done():
				m.complete(h, runCtx, content, runCtx.Err())
				return
			}
		}

		m.complete(h, runCtx, content, streamErr)
	}()

	return h
}

func (m *Manager) begin(ctx context.Context, agent Agent, input string, options ...StartOption) (*Handle, context.Context) {
	cfg := &startConfig{}
	for _, option := range options {
		option(cfg)
	}

	// A detached run must not inherit cancellation from the caller, or
	// "background" lasts only as long as the request that started it.
	base := ctx
	if cfg.detached {
		base = context.WithoutCancel(ctx)
	}

	var runCtx context.Context
	var cancel context.CancelFunc
	if cfg.timeout > 0 {
		runCtx, cancel = context.WithTimeout(base, cfg.timeout)
	} else {
		runCtx, cancel = context.WithCancel(base)
	}

	h := &Handle{
		id:        newID(),
		agentName: agent.GetName(),
		input:     input,
		cancel:    cancel,
		done:      make(chan struct{}),
		status:    StatusRunning,
		startedAt: time.Now(),
	}
	runCtx = WithRunID(runCtx, h.id)

	m.mu.Lock()
	m.runs[h.id] = h
	m.mu.Unlock()

	return h, runCtx
}

func (m *Manager) complete(h *Handle, runCtx context.Context, result string, err error) {
	status := StatusSucceeded
	switch {
	case errors.Is(runCtx.Err(), context.Canceled), errors.Is(runCtx.Err(), context.DeadlineExceeded):
		status = StatusCanceled
		if err == nil {
			err = runCtx.Err()
		}
	case err != nil:
		status = StatusFailed
	}

	h.finish(status, result, err)
	h.cancel() // release the context's resources
	m.retire(h.id)
}

// retire moves a finished run into the bounded retention list.
func (m *Manager) retire(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.retain == 0 {
		delete(m.runs, id)
		return
	}

	m.finished = append(m.finished, id)
	for len(m.finished) > m.retain {
		delete(m.runs, m.finished[0])
		m.finished = m.finished[1:]
	}
}

// Get returns the handle for a run ID.
func (m *Manager) Get(id string) (*Handle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.runs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return h, nil
}

// List returns a snapshot of every known run, in-flight and retained.
func (m *Manager) List() []Info {
	m.mu.RLock()
	handles := make([]*Handle, 0, len(m.runs))
	for _, h := range m.runs {
		handles = append(handles, h)
	}
	m.mu.RUnlock()

	infos := make([]Info, 0, len(handles))
	for _, h := range handles {
		infos = append(infos, h.Info())
	}
	return infos
}

// Active returns only the runs still in flight.
func (m *Manager) Active() []Info {
	var active []Info
	for _, info := range m.List() {
		if !info.Status.Terminal() {
			active = append(active, info)
		}
	}
	return active
}

// Cancel stops the run with the given ID.
func (m *Manager) Cancel(id string) error {
	h, err := m.Get(id)
	if err != nil {
		return err
	}
	h.Cancel()
	return nil
}

// CancelAll stops every run still in flight and returns how many it signalled.
func (m *Manager) CancelAll() int {
	m.mu.RLock()
	handles := make([]*Handle, 0, len(m.runs))
	for _, h := range m.runs {
		handles = append(handles, h)
	}
	m.mu.RUnlock()

	var n int
	for _, h := range handles {
		if !h.Info().Status.Terminal() {
			h.Cancel()
			n++
		}
	}
	return n
}

type contextKey string

const runIDKey contextKey = "run_id"

// WithRunID puts a run ID on the context.
func WithRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, runIDKey, id)
}

// FromContext returns the run ID on the context, if any.
//
// It returns the ID rather than the Handle deliberately: a Handle is mutable
// and reachable from arbitrary tool code, including MCP tools backed by remote
// processes, which could then mark a run succeeded or forge its result.
func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(runIDKey).(string)
	return id, ok
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	return "run_" + hex.EncodeToString(b[:])
}
