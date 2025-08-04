# Agent Microservices Documentation

This document describes the agent microservices functionality that allows you to create agents as microservices and communicate with them via gRPC.

## Overview

The agent microservices feature provides:

1. **gRPC Server Wrapper**: Convert any local agent into a gRPC microservice
2. **Remote Agent Client**: Connect to remote agent services using just a URL
3. **Unified Interface**: Use local and remote agents seamlessly together
4. **Service Management**: Tools for managing multiple microservices

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Main Application                             │
│                                                                 │
│  ┌─────────────┐  ┌─────────────────┐  ┌─────────────────────┐  │
│  │ Local Agent │  │ Remote Agent    │  │ Remote Agent        │  │
│  │             │  │ (localhost:8080)│  │ (service.com:443)   │  │
│  └─────────────┘  └─────────────────┘  └─────────────────────┘  │
│           │                │                         │          │
│           └────────────────┼─────────────────────────┘          │
│                            │                                    │
│  ┌─────────────────────────▼─────────────────────────────────┐  │
│  │            Orchestrator Agent                            │  │
│  │          (Delegates to subagents)                        │  │
│  └─────────────────────────┬─────────────────────────────────┘  │
└────────────────────────────┼────────────────────────────────────┘
                             │
                    ┌────────▼────────┐
                    │   User Input    │
                    └─────────────────┘
```

## Quick Start

### 1. Create a Local Agent Microservice

```go
package main

import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
    "github.com/Ingenimax/agent-sdk-go/pkg/microservice"
)

func main() {
    // Create a local agent
    mathAgent, _ := agent.NewAgent(
        agent.WithName("MathAgent"),
        agent.WithDescription("Mathematical calculations expert"),
        agent.WithLLM(openai.NewClient(apiKey)),
        agent.WithSystemPrompt("You are a math expert..."),
    )

    // Wrap as microservice
    service, _ := microservice.CreateMicroservice(mathAgent, microservice.Config{
        Port: 8080,
    })

    // Start the service
    service.Start()
}
```

### 2. Connect to Remote Agent

```go
// Create a remote agent connection
remoteAgent, _ := agent.NewAgent(
    agent.WithURL("localhost:8080"),
    agent.WithName("RemoteMathAgent"),
)

// Use it exactly like a local agent
result, _ := remoteAgent.Run(ctx, "What is 15 * 23?")
```

### 3. Mix Local and Remote Agents

```go
// Create local agent
localAgent, _ := agent.NewAgent(
    agent.WithName("LocalAgent"),
    agent.WithLLM(llm),
)

// Create remote agent
remoteAgent, _ := agent.NewAgent(
    agent.WithURL("localhost:8080"),
)

// Use both as subagents
orchestrator, _ := agent.NewAgent(
    agent.WithName("Orchestrator"),
    agent.WithLLM(llm),
    agent.WithAgents(localAgent, remoteAgent), // Mix local and remote!
)
```

## API Reference

### Core Functions

#### `agent.WithURL(url string) agent.Option`

Creates a remote agent that communicates via gRPC.

**Parameters:**
- `url`: The URL of the remote agent service (e.g., "localhost:8080", "service.example.com:443")

**Example:**
```go
remoteAgent, err := agent.NewAgent(
    agent.WithURL("localhost:8080"),
    agent.WithName("RemoteService"), // Optional, will be fetched from remote
)
```

#### `microservice.CreateMicroservice(agent *agent.Agent, config microservice.Config) (*microservice.AgentMicroservice, error)`

Wraps a local agent as a gRPC microservice.

**Parameters:**
- `agent`: The local agent to wrap
- `config`: Configuration for the microservice

**Example:**
```go
service, err := microservice.CreateMicroservice(agent, microservice.Config{
    Port:    8080,
    Timeout: 30 * time.Second,
})
```

### Agent Methods

#### `agent.IsRemote() bool`

Returns true if the agent is a remote agent.

#### `agent.GetRemoteURL() string`

Returns the URL of the remote agent (empty string if not remote).

#### `agent.Disconnect() error`

Closes the connection to a remote agent. Should be called when done with remote agents.

### Microservice Methods

#### `service.Start() error`

Starts the microservice server.

#### `service.Stop() error`

Stops the microservice server gracefully.

#### `service.IsRunning() bool`

Returns true if the microservice is currently running.

#### `service.GetPort() int`

Returns the port the microservice is running on.

#### `service.GetURL() string`

Returns the URL of the microservice (e.g., "localhost:8080").

#### `service.WaitForReady(timeout time.Duration) error`

Waits for the microservice to be ready to serve requests.

## gRPC Service Definition

The microservices expose the following gRPC service:

```protobuf
service AgentService {
    // Execute the agent
    rpc Run(RunRequest) returns (RunResponse);
    
    // Get agent metadata
    rpc GetMetadata(MetadataRequest) returns (MetadataResponse);
    
    // Health checks
    rpc Health(HealthRequest) returns (HealthResponse);
    rpc Ready(ReadinessRequest) returns (ReadinessResponse);
    
    // Execution plans (if supported)
    rpc GenerateExecutionPlan(PlanRequest) returns (PlanResponse);
    rpc ApproveExecutionPlan(ApprovalRequest) returns (ApprovalResponse);
}
```

## Service Management

### MicroserviceManager

Use the `MicroserviceManager` to manage multiple services:

```go
manager := microservice.NewMicroserviceManager()

// Register services
manager.RegisterService("math", mathService)
manager.RegisterService("code", codeService)

// Start all services
manager.StartAll()

// Stop specific service
manager.StopService("math")

// Stop all services
manager.StopAll()
```

## Configuration

### Microservice Config

```go
type Config struct {
    Port    int           // Port to run on (0 for auto-assign)
    Timeout time.Duration // Request timeout
}
```

### Remote Agent Config

Remote agents support the following configuration through the client:

```go
// Timeouts and retries are handled automatically
// Connection pooling and health checks are built-in
// Authentication support (coming soon)
```

## Error Handling

### Connection Errors

Remote agents automatically handle:
- Connection failures with retry logic
- Network timeouts
- Service unavailability

### Service Health

Microservices provide health endpoints:
- `/health` - Basic health check
- `/ready` - Readiness probe for Kubernetes

## Best Practices

### 1. Resource Management

Always disconnect from remote agents when done:

```go
defer remoteAgent.Disconnect()
```

### 2. Error Handling

Handle remote agent creation errors:

```go
remoteAgent, err := agent.NewAgent(agent.WithURL("localhost:8080"))
if err != nil {
    log.Printf("Failed to connect to remote agent: %v", err)
    // Fallback to local agent or return error
}
```

### 3. Service Discovery

Use service discovery for production deployments:

```go
// Instead of hardcoded URLs
remoteAgent, _ := agent.NewAgent(agent.WithURL("localhost:8080"))

// Use service discovery
serviceURL := discoveryClient.GetServiceURL("math-agent")
remoteAgent, _ := agent.NewAgent(agent.WithURL(serviceURL))
```

### 4. Load Balancing

For high availability, run multiple instances and use a load balancer:

```go
// Multiple instances of the same service
mathAgent1, _ := agent.NewAgent(agent.WithURL("math-service-1:8080"))
mathAgent2, _ := agent.NewAgent(agent.WithURL("math-service-2:8080"))

// Use a load balancing strategy
loadBalancer := NewLoadBalancer(mathAgent1, mathAgent2)
```

## Deployment

### Docker

Create a Dockerfile for your agent microservice:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o agent-service ./cmd/service

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/agent-service .
EXPOSE 8080
CMD ["./agent-service"]
```

### Kubernetes

Deploy using Kubernetes:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: math-agent
spec:
  replicas: 3
  selector:
    matchLabels:
      app: math-agent
  template:
    metadata:
      labels:
        app: math-agent
    spec:
      containers:
      - name: math-agent
        image: your-registry/math-agent:latest
        ports:
        - containerPort: 8080
        livenessProbe:
          grpc:
            port: 8080
        readinessProbe:
          grpc:
            port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: math-agent-service
spec:
  selector:
    app: math-agent
  ports:
  - port: 8080
    targetPort: 8080
```

## Security

### Authentication (Coming Soon)

Support for authentication is planned:

```go
// Future API
remoteAgent, _ := agent.NewAgent(
    agent.WithURL("secure-service.com:443"),
    agent.WithTLS(),
    agent.WithAuth(token),
)
```

### Network Security

- Use TLS for production deployments
- Implement proper network policies
- Use service mesh for inter-service communication

## Monitoring

### Metrics

Microservices automatically expose metrics:
- Request count
- Request duration
- Error rates
- Connection status

### Logging

Structured logging is built-in:
- Request/response logging
- Error logging
- Performance metrics

### Tracing

Distributed tracing is supported:
- OpenTelemetry integration
- Request correlation IDs
- Cross-service tracing

## Troubleshooting

### Common Issues

1. **Connection refused**: Ensure the remote service is running
2. **Timeout errors**: Check network connectivity and service health
3. **Authentication errors**: Verify credentials and permissions
4. **Port conflicts**: Use different ports or auto-assignment

### Debug Mode

Enable debug logging:

```go
// Set environment variable
os.Setenv("AGENT_LOG_LEVEL", "debug")
```

### Health Checks

Test service health:

```bash
# Check if service is responding
grpcurl -plaintext localhost:8080 agent.AgentService/Health

# Test agent execution
grpcurl -plaintext -d '{"input":"test"}' localhost:8080 agent.AgentService/Run
```

## Examples

See the `examples/microservices/` directory for complete examples:

- `basic_microservice/` - Simple microservice setup
- `remote_client/` - Connecting to remote agents
- `mixed_agents/` - Local and remote agents together
- `service_manager/` - Managing multiple services

## Migration Guide

### From Direct Agent Usage

**Before:**
```go
agent1.Run(ctx, input)
agent2.Run(ctx, input)
```

**After:**
```go
orchestrator := agent.NewAgent(
    agent.WithAgents(agent1, remoteAgent2),
)
orchestrator.Run(ctx, input) // Automatically delegates
```

### From Manual Service Management

**Before:**
```go
// Manual gRPC server setup
server := grpc.NewServer()
pb.RegisterAgentServiceServer(server, agentServer)
server.Serve(listener)
```

**After:**
```go
// Simplified microservice creation
service, _ := microservice.CreateMicroservice(agent, config)
service.Start()
```