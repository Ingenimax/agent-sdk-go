package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/grpc/server"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	fmt.Println("=== Testing gRPC Health Check Implementation ===")
	
	// Test 1: Basic gRPC health server
	fmt.Println("\n1. Testing basic gRPC health server...")
	if err := testBasicHealthServer(); err != nil {
		log.Printf("Basic health server test failed: %v", err)
	} else {
		fmt.Println("✅ Basic health server test passed")
	}
	
	// Test 2: Agent server health check
	fmt.Println("\n2. Testing agent server health check...")
	if err := testAgentServerHealth(); err != nil {
		log.Printf("Agent server health test failed: %v", err)
	} else {
		fmt.Println("✅ Agent server health test passed")
	}
}

// testBasicHealthServer tests a minimal gRPC server with just health service
func testBasicHealthServer() error {
	// Create a basic gRPC server
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("Basic server listening on port %d\n", port)
	
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	
	// Register health service
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	
	// Set health status
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	
	// Start server in background
	serverDone := make(chan error, 1)
	go func() {
		fmt.Printf("Basic server starting to serve on port %d\n", port)
		serverDone <- grpcServer.Serve(listener)
	}()
	
	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)
	
	// Test health check
	err = testHealthEndpoint(port)
	
	// Cleanup
	grpcServer.Stop()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
	}
	
	return err
}

// testAgentServerHealth tests the actual agent server implementation
func testAgentServerHealth() error {
	// Create a minimal LLM (doesn't need to work, just needs to exist)
	llm := openai.NewClient("fake-key")
	
	// Create agent
	testAgent, err := agent.NewAgent(
		agent.WithName("TestAgent"),
		agent.WithDescription("Test agent for health check"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt("Test prompt"),
	)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	
	// Create agent server
	agentServer := server.NewAgentServer(testAgent)
	
	// Create listener
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("Agent server listening on port %d\n", port)
	
	// Start server in background
	serverDone := make(chan error, 1)
	go func() {
		fmt.Printf("Agent server starting to serve on port %d\n", port)
		serverDone <- agentServer.StartWithListener(listener)
	}()
	
	// Give server more time to start since it's more complex
	time.Sleep(500 * time.Millisecond)
	
	// Test health check
	err = testHealthEndpoint(port)
	
	// Cleanup
	agentServer.Stop()
	select {
	case <-serverDone:
	case <-time.After(time.Second):
	}
	
	return err
}

// testHealthEndpoint tests the health endpoint on a given port
func testHealthEndpoint(port int) error {
	fmt.Printf("Testing health endpoint on port %d...\n", port)
	
	// Create gRPC connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()
	
	// Test health check
	healthClient := grpc_health_v1.NewHealthClient(conn)
	
	fmt.Printf("Calling health check...\n")
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "", // Check overall server health
	})
	
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	
	fmt.Printf("Health check response: %v\n", resp.Status)
	
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("expected SERVING status, got %v", resp.Status)
	}
	
	return nil
}