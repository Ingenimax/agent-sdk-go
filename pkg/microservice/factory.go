package microservice

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/server"
)

// AgentMicroservice represents a microservice wrapping an agent
type AgentMicroservice struct {
	agent      *agent.Agent
	server     *server.AgentServer
	port       int
	running    bool
	mu         sync.RWMutex
	cancelFunc context.CancelFunc
}

// Config holds configuration for creating an agent microservice
type Config struct {
	Port    int           // Port to run the service on (0 for auto-assign)
	Timeout time.Duration // Request timeout
}

// CreateMicroservice creates a new agent microservice
func CreateMicroservice(agent *agent.Agent, config Config) (*AgentMicroservice, error) {
	if agent == nil {
		return nil, fmt.Errorf("agent cannot be nil")
	}

	if agent.IsRemote() {
		return nil, fmt.Errorf("cannot create microservice from remote agent")
	}

	if config.Port < 0 || config.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", config.Port)
	}

	server := server.NewAgentServer(agent)

	return &AgentMicroservice{
		agent:  agent,
		server: server,
		port:   config.Port,
	}, nil
}

// Start starts the microservice
func (m *AgentMicroservice) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("microservice is already running")
	}

	// Create a listener first to get the actual port
	addr := fmt.Sprintf(":%d", m.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", m.port, err)
	}
	
	// Update port if it was auto-assigned (port 0)
	if m.port == 0 {
		m.port = listener.Addr().(*net.TCPAddr).Port
	}

	// Create a context for the server
	_, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	// Mark as running now that we have successfully bound to the port
	m.running = true
	
	// Start the server in a goroutine
	go func() {
		defer func() {
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
		}()
		
		err := m.server.StartWithListener(listener)
		if err != nil {
			fmt.Printf("Agent server error: %v\n", err)
		}
	}()

	fmt.Printf("Agent microservice '%s' started on port %d\n", m.agent.GetName(), m.port)
	return nil
}

// Stop stops the microservice
func (m *AgentMicroservice) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil // Already stopped
	}

	// Stop the gRPC server
	m.server.Stop()

	// Cancel the context
	if m.cancelFunc != nil {
		m.cancelFunc()
	}

	m.running = false
	fmt.Printf("Agent microservice '%s' stopped\n", m.agent.GetName())
	return nil
}

// IsRunning returns true if the microservice is currently running
func (m *AgentMicroservice) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// GetPort returns the port the microservice is running on
func (m *AgentMicroservice) GetPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.port
}

// GetURL returns the URL of the microservice
func (m *AgentMicroservice) GetURL() string {
	return fmt.Sprintf("localhost:%d", m.GetPort())
}

// GetAgent returns the underlying agent
func (m *AgentMicroservice) GetAgent() *agent.Agent {
	return m.agent
}

// WaitForReady waits for the microservice to be ready to serve requests
func (m *AgentMicroservice) WaitForReady(timeout time.Duration) error {
	// Wait a moment for the service to mark itself as running
	deadline := time.Now().Add(timeout)
	
	// First, wait for the service to be marked as running
	for time.Now().Before(deadline) {
		if m.IsRunning() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	
	// If still not running after timeout, return error
	if !m.IsRunning() {
		return fmt.Errorf("microservice failed to start within %v", timeout)
	}
	
	// Now try to connect to the service
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", m.port), time.Second)
		if err == nil {
			if closeErr := conn.Close(); closeErr != nil {
				// Log the close error but don't fail the whole operation
				fmt.Printf("Warning: failed to close connection: %v\n", closeErr)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("microservice not ready after %v", timeout)
}

// MicroserviceManager manages multiple agent microservices
type MicroserviceManager struct {
	services map[string]*AgentMicroservice
	mu       sync.RWMutex
}

// NewMicroserviceManager creates a new microservice manager
func NewMicroserviceManager() *MicroserviceManager {
	return &MicroserviceManager{
		services: make(map[string]*AgentMicroservice),
	}
}

// RegisterService registers a microservice with the manager
func (mm *MicroserviceManager) RegisterService(name string, service *AgentMicroservice) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.services[name]; exists {
		return fmt.Errorf("service with name %s already exists", name)
	}

	mm.services[name] = service
	return nil
}

// StartService starts a service by name
func (mm *MicroserviceManager) StartService(name string) error {
	mm.mu.RLock()
	service, exists := mm.services[name]
	mm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	return service.Start()
}

// StopService stops a service by name
func (mm *MicroserviceManager) StopService(name string) error {
	mm.mu.RLock()
	service, exists := mm.services[name]
	mm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not found", name)
	}

	return service.Stop()
}

// StartAll starts all registered services
func (mm *MicroserviceManager) StartAll() error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	for name, service := range mm.services {
		if err := service.Start(); err != nil {
			return fmt.Errorf("failed to start service %s: %w", name, err)
		}
	}

	return nil
}

// StopAll stops all running services
func (mm *MicroserviceManager) StopAll() error {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	var lastErr error
	for name, service := range mm.services {
		if err := service.Stop(); err != nil {
			lastErr = fmt.Errorf("failed to stop service %s: %w", name, err)
		}
	}

	return lastErr
}

// GetService returns a service by name
func (mm *MicroserviceManager) GetService(name string) (*AgentMicroservice, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	service, exists := mm.services[name]
	return service, exists
}

// ListServices returns all registered service names
func (mm *MicroserviceManager) ListServices() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	names := make([]string, 0, len(mm.services))
	for name := range mm.services {
		names = append(names, name)
	}

	return names
}