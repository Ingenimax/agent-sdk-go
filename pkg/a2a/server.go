package a2a

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// Server wraps an agent-sdk-go agent and exposes it as an A2A-compliant HTTP server.
type Server struct {
	agent     AgentAdapter
	card      *a2a.AgentCard
	handler   a2asrv.RequestHandler
	mux       *http.ServeMux
	addr      string
	basePath  string
	logger    logging.Logger
	taskStore TaskStore
}

// NewServer creates a new A2A server that serves the given agent.
// The agentCard describes the agent's capabilities to A2A clients.
func NewServer(agent AgentAdapter, agentCard *a2a.AgentCard, opts ...ServerOption) *Server {
	s := &Server{
		agent:    agent,
		card:     agentCard,
		addr:     ":0",
		basePath: "/",
		logger:   logging.New(),
	}
	for _, opt := range opts {
		opt(s)
	}

	if s.taskStore == nil {
		s.taskStore = NewInMemoryTaskStore()
	}

	executor := newAgentExecutor(agent, s.logger)

	var handlerOpts []a2asrv.RequestHandlerOption
	s.handler = a2asrv.NewHandler(executor, handlerOpts...)

	s.mux = http.NewServeMux()
	s.mux.Handle(s.basePath, a2asrv.NewJSONRPCHandler(s.handler))
	s.mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))

	return s
}

// Handler returns the http.Handler so callers can mount it on their own server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start starts the A2A server and blocks until the context is canceled.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("a2a server: failed to listen on %s: %w", s.addr, err)
	}

	s.logger.Info(ctx, "A2A server starting", map[string]interface{}{
		"address":    listener.Addr().String(),
		"agent":      s.agent.GetName(),
		"agent_card": s.card.Name,
		"base_path":  s.basePath,
	})

	srv := &http.Server{
		Handler: s.mux,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		s.logger.Info(ctx, "A2A server shutting down", nil)
		return srv.Close()
	case err := <-errCh:
		return err
	}
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.addr
}
