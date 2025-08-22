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
)

func main() {
	fmt.Println("=== Testing Simple gRPC Health Server ===")
	
	// This tests the most basic gRPC health implementation to see if it works
	
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("Simple gRPC server starting on port %d\n", port)
	
	// Create server
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	
	// Register health service
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	
	// Set health status
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	fmt.Println("Health status set to SERVING")
	
	// Start server
	serverReady := make(chan struct{})
	go func() {
		close(serverReady) // Signal that we're about to start serving
		fmt.Println("gRPC server Serve() called")
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()
	
	// Wait for server to start
	<-serverReady
	fmt.Println("Server goroutine started")
	
	// Give it a moment to actually start serving
	time.Sleep(200 * time.Millisecond)
	
	// Now test the health check
	fmt.Println("Testing health check...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()
	
	healthClient := grpc_health_v1.NewHealthClient(conn)
	
	fmt.Println("Calling health check...")
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: "",
	})
	
	if err != nil {
		log.Fatalf("Health check failed: %v", err)
	}
	
	fmt.Printf("✅ Health check succeeded! Status: %v\n", resp.Status)
	
	// Cleanup
	grpcServer.Stop()
	fmt.Println("Server stopped")
}