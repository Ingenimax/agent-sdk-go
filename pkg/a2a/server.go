package a2a

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

const (
	defaultShutdownTimeout    = 30 * time.Second
	defaultReadHeaderTimeout  = 10 * time.Second
)

// Server wraps an agent-sdk-go agent and exposes it as an A2A-compliant HTTP server.
type Server struct {
	agent             AgentAdapter
	card              *a2a.AgentCard
	handler           a2asrv.RequestHandler
	mux               *http.ServeMux
	builtHandler      http.Handler
	addr              string
	resolvedAddr      string
	addrMu            sync.RWMutex
	basePath          string
	logger            logging.Logger
	shutdownTimeout   time.Duration
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	middlewares       []func(http.Handler) http.Handler
}

// NewServer creates a new A2A server that serves the given agent.
// The agentCard describes the agent's capabilities to A2A clients.
// It panics if agent or agentCard is nil.
func NewServer(agent AgentAdapter, agentCard *a2a.AgentCard, opts ...ServerOption) *Server {
	if agent == nil {
		panic("a2a: NewServer requires a non-nil agent")
	}
	if agentCard == nil {
		panic("a2a: NewServer requires a non-nil agentCard")
	}
	s := &Server{
		agent:             agent,
		card:              agentCard,
		addr:              ":0",
		basePath:          "/",
		logger:            logging.New(),
		shutdownTimeout:   defaultShutdownTimeout,
		readHeaderTimeout: defaultReadHeaderTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}

	executor := newAgentExecutor(agent, s.logger)
	s.handler = a2asrv.NewHandler(executor)

	s.mux = http.NewServeMux()
	s.mux.Handle(s.basePath, a2asrv.NewJSONRPCHandler(s.handler))
	s.mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))

	// Build and cache the final handler with middleware applied.
	var h http.Handler = s.mux
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	s.builtHandler = h

	return s
}

// Handler returns the http.Handler so callers can mount it on their own server.
// Middleware is applied in the order it was added. The handler is built once
// during NewServer and cached for subsequent calls.
func (s *Server) Handler() http.Handler {
	return s.builtHandler
}

// Start starts the A2A server and blocks until the context is canceled.
// On cancellation, the server performs a graceful shutdown within the configured timeout.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("a2a server: failed to listen on %s: %w", s.addr, err)
	}
	s.addrMu.Lock()
	s.resolvedAddr = listener.Addr().String()
	s.addrMu.Unlock()

	s.logger.Info(ctx, "A2A server starting", map[string]interface{}{
		"address":          s.resolvedAddr,
		"agent":            s.agent.GetName(),
		"agent_card":       s.card.Name,
		"base_path":        s.basePath,
		"shutdown_timeout": s.shutdownTimeout.String(),
	})

	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: s.readHeaderTimeout,
		ReadTimeout:       s.readTimeout,
		WriteTimeout:      s.writeTimeout,
		IdleTimeout:       s.idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		s.logger.Info(ctx, "A2A server shutting down gracefully", map[string]interface{}{
			"timeout": s.shutdownTimeout.String(),
		})
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		// Drain the serve goroutine; Shutdown causes Serve to return
		// ErrServerClosed which is expected during graceful shutdown.
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Addr returns the resolved listen address after Start has been called.
// Before Start, it returns the configured address.
func (s *Server) Addr() string {
	s.addrMu.RLock()
	resolved := s.resolvedAddr
	s.addrMu.RUnlock()
	if resolved != "" {
		return resolved
	}
	return s.addr
}
