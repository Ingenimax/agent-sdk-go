package sandbox

import (
	"context"
	"testing"
)

func TestPool_Acquire_RoundRobin(t *testing.T) {
	containers := []Container{
		{ID: "abc123", Name: "sandbox-0", Ready: true},
		{ID: "def456", Name: "sandbox-1", Ready: true},
	}
	p := &Pool{containers: containers}

	c1, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c2, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c3, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c1.ID != "abc123" {
		t.Errorf("expected first container, got %s", c1.ID)
	}
	if c2.ID != "def456" {
		t.Errorf("expected second container, got %s", c2.ID)
	}
	if c3.ID != "abc123" {
		t.Errorf("expected round-robin back to first, got %s", c3.ID)
	}
}

func TestPool_Acquire_EmptyPool(t *testing.T) {
	p := &Pool{containers: nil}
	_, err := p.Acquire(context.Background())
	if err == nil {
		t.Error("expected error for empty pool")
	}
}

func TestPool_Acquire_UnhealthyContainer(t *testing.T) {
	containers := []Container{
		{ID: "abc123", Name: "sandbox-0", Ready: false},
	}
	p := &Pool{containers: containers}
	_, err := p.Acquire(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy container")
	}
}

func TestPool_MarkUnhealthy(t *testing.T) {
	containers := []Container{
		{ID: "abc123", Name: "sandbox-0", Ready: true},
	}
	p := &Pool{containers: containers}
	p.MarkUnhealthy("abc123")

	_, err := p.Acquire(context.Background())
	if err == nil {
		t.Error("expected error after marking container unhealthy")
	}
}

func TestPool_Close(t *testing.T) {
	var closed []string
	closeFn := func(ctx context.Context, id string) error {
		closed = append(closed, id)
		return nil
	}
	containers := []Container{
		{ID: "abc123", Name: "sandbox-0", Ready: true},
		{ID: "def456", Name: "sandbox-1", Ready: true},
	}
	p := &Pool{containers: containers, closeFn: closeFn}

	if err := p.Close(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(closed) != 2 {
		t.Errorf("expected 2 containers closed, got %d", len(closed))
	}
}
