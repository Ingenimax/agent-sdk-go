package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
)

func main() {
	// Example demonstrating how to configure remote agent timeout

	// Method 1: Using WithRemoteTimeout option for custom timeout
	remoteAgent, err := agent.NewAgent(
		agent.WithName("RemoteAgent"),
		agent.WithDescription("An agent running on a remote server with custom timeout"),
		agent.WithURL("localhost:50051"),
		agent.WithRemoteTimeout(10*time.Minute), // Set custom timeout to 10 minutes
	)
	if err != nil {
		log.Fatalf("Failed to create remote agent: %v", err)
	}
	defer func() {
		_ = remoteAgent.Disconnect()
	}()

	// Method 2: Using default timeout (5 minutes)
	remoteAgent2, err := agent.NewAgent(
		agent.WithName("RemoteAgent2"),
		agent.WithDescription("An agent using default 5-minute timeout"),
		agent.WithURL("localhost:50052"),
		// No WithRemoteTimeout specified, will use default 5 minutes
	)
	if err != nil {
		log.Fatalf("Failed to create second remote agent: %v", err)
	}
	defer func() {
		_ = remoteAgent2.Disconnect()
	}()

	// Use the remote agents
	ctx := context.Background()

	// This will have a 10-minute timeout for operations
	result, err := remoteAgent.Run(ctx, "Perform a long-running task that might take several minutes")
	if err != nil {
		log.Printf("Error running remote agent: %v", err)
	} else {
		fmt.Printf("Remote agent result: %s\n", result)
	}

	// This will have the default 5-minute timeout
	result2, err := remoteAgent2.Run(ctx, "Another long-running task")
	if err != nil {
		log.Printf("Error running second remote agent: %v", err)
	} else {
		fmt.Printf("Second remote agent result: %s\n", result2)
	}
}
