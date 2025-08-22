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
	fmt.Println("=== Testing gRPC Client Creation Methods ===")
	
	// Start a simple gRPC server
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	
	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("gRPC server starting on port %d\n", port)
	
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()
	
	// Wait longer for server to be ready
	time.Sleep(1 * time.Second)
	
	// Test Method 1: grpc.NewClient (newer API)
	fmt.Println("\nMethod 1: Testing grpc.NewClient...")
	if err := testWithNewClient(port); err != nil {
		fmt.Printf("❌ grpc.NewClient failed: %v\n", err)
	} else {
		fmt.Println("✅ grpc.NewClient succeeded")
	}
	
	// Test Method 2: grpc.Dial (older API)
	fmt.Println("\nMethod 2: Testing grpc.Dial...")
	if err := testWithDial(port); err != nil {
		fmt.Printf("❌ grpc.Dial failed: %v\n", err)
	} else {
		fmt.Println("✅ grpc.Dial succeeded")
	}
	
	// Test Method 3: Different timeout values
	fmt.Println("\nMethod 3: Testing different timeout values...")
	timeouts := []time.Duration{1 * time.Second, 5 * time.Second, 10 * time.Second}
	for _, timeout := range timeouts {
		if err := testWithTimeout(port, timeout); err != nil {
			fmt.Printf("❌ Timeout %v failed: %v\n", timeout, err)
		} else {
			fmt.Printf("✅ Timeout %v succeeded\n", timeout)
			break
		}
	}
	
	grpcServer.Stop()
	fmt.Println("\nTest completed")
}

func testWithNewClient(port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	conn, err := grpc.NewClient(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to create connection: %w", err)
	}
	defer conn.Close()
	
	healthClient := grpc_health_v1.NewHealthClient(conn)
	_, err = healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
	return err
}

func testWithDial(port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Wait for connection to be established
	)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()
	
	healthClient := grpc_health_v1.NewHealthClient(conn)
	_, err = healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
	return err
}

func testWithTimeout(port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(timeout),
	)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()
	
	healthClient := grpc_health_v1.NewHealthClient(conn)
	_, err = healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
	return err
}