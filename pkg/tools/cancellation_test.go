package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// blockingSubAgent runs until its context is cancelled, then reports which
// happened. It lets the test observe whether cancellation actually reached the
// sub-agent rather than inferring it from timing.
type blockingSubAgent struct {
	started  chan struct{}
	finished chan error
}

func newBlockingSubAgent() *blockingSubAgent {
	return &blockingSubAgent{
		started:  make(chan struct{}, 1),
		finished: make(chan error, 1),
	}
}

func (s *blockingSubAgent) GetName() string        { return "blocking" }
func (s *blockingSubAgent) GetDescription() string { return "blocks until its context ends" }

func (s *blockingSubAgent) Run(ctx context.Context, _ string) (string, error) {
	s.started <- struct{}{}
	<-ctx.Done()
	s.finished <- ctx.Err()
	return "", ctx.Err()
}

func (s *blockingSubAgent) RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error) {
	content, err := s.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	return &interfaces.AgentResponse{Content: content}, nil
}

func (s *blockingSubAgent) RunStream(ctx context.Context, input string) (<-chan interfaces.AgentStreamEvent, error) {
	ch := make(chan interfaces.AgentStreamEvent)
	go func() {
		defer close(ch)
		_, _ = s.Run(ctx, input)
	}()
	return ch, nil
}

// TestAgentTool_ParentCancellationStopsSubAgent guards the fix for a sub-agent
// that outlived the run which spawned it.
//
// AgentTool.Execute used to do this when the parent deadline was earlier than
// its own timeout:
//
//	newCtx := context.WithoutCancel(ctx)
//	ctx, cancel = context.WithTimeout(newCtx, at.timeout)
//
// The comment claimed this "removes the parent's deadline while preserving
// values". context.WithoutCancel also severs cancellation propagation, which
// the comment did not mention. With the default 30 minute sub-agent timeout,
// cancelling a parent run left the sub-agent running for up to half an hour,
// still writing into shared memory, after the caller was gone.
//
// The sub-agent is now always a child of the caller's context.
func TestAgentTool_ParentCancellationStopsSubAgent(t *testing.T) {
	sub := newBlockingSubAgent()

	// A long tool timeout, like the 30 minute default, so the old code would
	// have taken the WithoutCancel branch and detached.
	tool := NewAgentTool(sub).WithTimeout(30 * time.Minute)

	// A parent deadline far shorter than the tool timeout: exactly the
	// condition that used to trigger detachment.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = tool.Execute(ctx, `{"query":"work"}`)
	}()

	select {
	case <-sub.started:
	case <-time.After(5 * time.Second):
		t.Fatal("sub-agent never started")
	}

	select {
	case err := <-sub.finished:
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("sub-agent context ended with %v, want deadline exceeded or canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent deadline passed but the sub-agent was still running: " +
			"cancellation did not propagate, so the sub-agent has outlived its parent run")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("AgentTool.Execute did not return after its context ended")
	}
}

// TestAgentTool_ExplicitCancelStopsSubAgent covers the same invariant for an
// explicitly cancelled parent rather than an expiring deadline. A parent with
// no deadline at all took the other branch of the old code, so this pins the
// behaviour of both.
func TestAgentTool_ExplicitCancelStopsSubAgent(t *testing.T) {
	sub := newBlockingSubAgent()
	tool := NewAgentTool(sub).WithTimeout(30 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_, _ = tool.Execute(ctx, `{"query":"work"}`)
	}()

	select {
	case <-sub.started:
	case <-time.After(5 * time.Second):
		t.Fatal("sub-agent never started")
	}

	cancel()

	select {
	case err := <-sub.finished:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("sub-agent context ended with %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent was cancelled but the sub-agent kept running")
	}
}
