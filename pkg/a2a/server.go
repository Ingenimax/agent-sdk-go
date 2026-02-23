package a2a

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

const defaultShutdownTimeout = 30 * time.Second

// Server wraps an agent-sdk-go agent and exposes it as an A2A-compliant HTTP server.
type Server struct {
	agent           AgentAdapter
	card            *a2a.AgentCard
	handler         a2asrv.RequestHandler
	mux             *http.ServeMux
	addr            string
	resolvedAddr    string
	basePath        string
	logger          logging.Logger
	shutdownTimeout time.Duration
	middlewares     []func(http.Handler) http.Handler
}

// NewServer creates a new A2A server that serves the given agent.
// The agentCard describes the agent's capabilities to A2A clients.
func NewServer(agent AgentAdapter, agentCard *a2a.AgentCard, opts ...ServerOption) *Server {
	s := &Server{
		agent:           agent,
		card:            agentCard,
		addr:            ":0",
		basePath:        "/",
		logger:          logging.New(),
		shutdownTimeout: defaultShutdownTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}

	executor := newAgentExecutor(agent, s.logger)
	s.handler = a2asrv.NewHandler(executor)

	s.mux = http.NewServeMux()
	s.mux.Handle(s.basePath, a2asrv.NewJSONRPCHandler(s.handler))
	s.mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))

	return s
}

// Handler returns the http.Handler so callers can mount it on their own server.
// Middleware is applied in the order it was added.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	return h
}

// Start starts the A2A server and blocks until the context is canceled.
// On cancellation, the server performs a graceful shutdown within the configured timeout.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("a2a server: failed to listen on %s: %w", s.addr, err)
	}
	s.resolvedAddr = listener.Addr().String()

	s.logger.Info(ctx, "A2A server starting", map[string]interface{}{
		"address":          s.resolvedAddr,
		"agent":            s.agent.GetName(),
		"agent_card":       s.card.Name,
		"base_path":        s.basePath,
		"shutdown_timeout": s.shutdownTimeout.String(),
	})

	srv := &http.Server{
		Handler: s.Handler(),
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Addr returns the resolved listen address after Start has been called.
// Before Start, it returns the configured address.
func (s *Server) Addr() string {
	if s.resolvedAddr != "" {
		return s.resolvedAddr
	}
	return s.addr
}
