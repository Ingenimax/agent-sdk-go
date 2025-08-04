package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/pb"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// RemoteAgentClient handles communication with remote agents via gRPC
type RemoteAgentClient struct {
	url        string
	conn       *grpc.ClientConn
	client     pb.AgentServiceClient
	timeout    time.Duration
	retryCount int
}

// RemoteAgentConfig configures the remote agent client
type RemoteAgentConfig struct {
	URL        string
	Timeout    time.Duration
	RetryCount int
}

// NewRemoteAgentClient creates a new remote agent client
func NewRemoteAgentClient(config RemoteAgentConfig) *RemoteAgentClient {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}

	return &RemoteAgentClient{
		url:        config.URL,
		timeout:    config.Timeout,
		retryCount: config.RetryCount,
	}
}

// Connect establishes a connection to the remote agent service
func (r *RemoteAgentClient) Connect() error {
	if r.conn != nil {
		return nil // Already connected
	}

	conn, err := grpc.Dial(r.url, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTimeout(r.timeout),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", r.url, err)
	}

	r.conn = conn
	r.client = pb.NewAgentServiceClient(conn)

	// Test the connection with a health check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = r.client.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		r.conn.Close()
		r.conn = nil
		r.client = nil
		return fmt.Errorf("health check failed for %s: %w", r.url, err)
	}

	return nil
}

// Disconnect closes the connection to the remote agent service
func (r *RemoteAgentClient) Disconnect() error {
	if r.conn != nil {
		err := r.conn.Close()
		r.conn = nil
		r.client = nil
		return err
	}
	return nil
}

// Run executes the remote agent with the given input
func (r *RemoteAgentClient) Run(ctx context.Context, input string) (string, error) {
	if err := r.ensureConnected(); err != nil {
		return "", err
	}

	// Create request
	req := &pb.RunRequest{
		Input: input,
		Context: make(map[string]string),
	}

	// Add org_id from context if available
	if orgID, _ := multitenancy.GetOrgID(ctx); orgID != "" {
		req.OrgId = orgID
	}

	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// Execute with retry logic
	var lastErr error
	for attempt := 0; attempt < r.retryCount; attempt++ {
		resp, err := r.client.Run(ctx, req)
		if err != nil {
			lastErr = err
			// Exponential backoff
			if attempt < r.retryCount-1 {
				backoff := time.Duration(attempt+1) * time.Second
				time.Sleep(backoff)
			}
			continue
		}

		if resp.Error != "" {
			return "", fmt.Errorf("remote agent error: %s", resp.Error)
		}

		return resp.Output, nil
	}

	return "", fmt.Errorf("failed after %d attempts, last error: %w", r.retryCount, lastErr)
}

// GetMetadata retrieves metadata from the remote agent
func (r *RemoteAgentClient) GetMetadata(ctx context.Context) (*pb.MetadataResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return r.client.GetMetadata(ctx, &pb.MetadataRequest{})
}

// GetCapabilities retrieves capabilities from the remote agent
func (r *RemoteAgentClient) GetCapabilities(ctx context.Context) (*pb.CapabilitiesResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return r.client.GetCapabilities(ctx, &pb.CapabilitiesRequest{})
}

// Health checks the health of the remote agent service
func (r *RemoteAgentClient) Health(ctx context.Context) (*pb.HealthResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.client.Health(ctx, &pb.HealthRequest{})
}

// Ready checks if the remote agent service is ready
func (r *RemoteAgentClient) Ready(ctx context.Context) (*pb.ReadinessResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.client.Ready(ctx, &pb.ReadinessRequest{})
}

// GenerateExecutionPlan generates an execution plan via the remote agent
func (r *RemoteAgentClient) GenerateExecutionPlan(ctx context.Context, input string) (*pb.PlanResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	req := &pb.PlanRequest{
		Input:   input,
		Context: make(map[string]string),
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.client.GenerateExecutionPlan(ctx, req)
}

// ApproveExecutionPlan approves an execution plan via the remote agent
func (r *RemoteAgentClient) ApproveExecutionPlan(ctx context.Context, planID string, approved bool, modifications string) (*pb.ApprovalResponse, error) {
	if err := r.ensureConnected(); err != nil {
		return nil, err
	}

	req := &pb.ApprovalRequest{
		PlanId:        planID,
		Approved:      approved,
		Modifications: modifications,
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	return r.client.ApproveExecutionPlan(ctx, req)
}

// ensureConnected ensures that the client is connected to the remote service
func (r *RemoteAgentClient) ensureConnected() error {
	if r.conn == nil || r.client == nil {
		return r.Connect()
	}
	return nil
}

// IsConnected returns true if the client is connected
func (r *RemoteAgentClient) IsConnected() bool {
	return r.conn != nil && r.client != nil
}

// GetURL returns the URL of the remote agent
func (r *RemoteAgentClient) GetURL() string {
	return r.url
}

// SetTimeout sets the timeout for requests
func (r *RemoteAgentClient) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}

// SetRetryCount sets the number of retries for failed requests
func (r *RemoteAgentClient) SetRetryCount(count int) {
	r.retryCount = count
}