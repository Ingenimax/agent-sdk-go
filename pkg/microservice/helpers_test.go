package microservice

import (
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/server"
)

// startGRPCAgent serves a on a localhost gRPC port and returns its address.
func startGRPCAgent(t *testing.T, a *agent.Agent) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := server.NewAgentServer(a)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.StartWithListener(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		<-done
	})
	return lis.Addr().String()
}

// newRemoteAgent returns an agent that proxies to the gRPC server at addr.
func newRemoteAgent(t *testing.T, addr string) *agent.Agent {
	t.Helper()
	a, err := agent.NewAgent(agent.WithURL(addr), agent.WithRemoteTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("NewAgent remote: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect() })
	if !a.IsRemote() {
		t.Fatal("agent is not remote")
	}
	return a
}

// freePort returns a localhost port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	if err := lis.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// waitForHTTP polls url until it answers or the deadline passes.
func waitForHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) // #nosec G107 - localhost test URL
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not start", url)
}

// baseURL returns the localhost URL for port.
func baseURL(port int) string {
	return "http://127.0.0.1:" + strconv.Itoa(port)
}
