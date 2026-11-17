package keenable

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newCountingServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"t","url":"https://example.com","snippet":"s"}]}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCacheIsBounded: every distinct query an LLM writes becomes a cache key,
// so a long-running agent must not keep them all.
func TestCacheIsBounded(t *testing.T) {
	tool := New("", WithBaseURL(newCountingServer(t).URL))
	for i := 0; i < maxCacheEntries+500; i++ {
		if _, err := tool.Run(context.Background(), fmt.Sprintf("query %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(tool.cache); n > maxCacheEntries {
		t.Errorf("cache holds %d entries, want at most %d", n, maxCacheEntries)
	}
	// The most recent query must still be served from the cache.
	last := fmt.Sprintf("%d\x00query %d", defaultNumResults, maxCacheEntries+499)
	if _, ok := tool.cache[last]; !ok {
		t.Error("most recent entry was evicted")
	}
}

func TestExpiredEntriesAreDropped(t *testing.T) {
	now := time.Now()
	tool := New("", WithBaseURL(newCountingServer(t).URL))
	tool.now = func() time.Time { return now }

	for i := 0; i < 100; i++ {
		if _, err := tool.Run(context.Background(), fmt.Sprintf("query %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(cacheTTL + time.Minute)

	// A repeat of an expired query replaces its entry rather than serving it.
	if _, err := tool.Run(context.Background(), "query 0"); err != nil {
		t.Fatal(err)
	}
	if got := tool.cache[fmt.Sprintf("%d\x00query 0", defaultNumResults)].timestamp; !got.Equal(now) {
		t.Errorf("expired entry was served instead of refreshed (timestamp %v)", got)
	}

	// Filling the cache sweeps the expired entries before evicting live ones.
	for i := 0; i < maxCacheEntries; i++ {
		if _, err := tool.Run(context.Background(), fmt.Sprintf("fresh %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for key, entry := range tool.cache {
		if now.Sub(entry.timestamp) >= cacheTTL {
			t.Fatalf("expired entry %q still cached", key)
		}
	}
}
