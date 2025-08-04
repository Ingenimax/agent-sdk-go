# Agent Microservices Examples

This directory contains examples of how to use the agent microservices functionality in the agent-sdk-go.

## Overview

The agent microservices feature allows you to:

1. **Wrap existing agents as gRPC microservices** - Take any local agent and expose it via gRPC
2. **Create remote agents** - Connect to remote agent services using just a URL
3. **Mix local and remote agents** - Seamlessly use both local and remote agents as subagents

## Examples

### 1. Basic Microservice (`basic_microservice/`)

Shows how to:
- Create a local agent
- Wrap it as a microservice
- Start the service on a specific port

### 2. Remote Agent Client (`remote_client/`)

Demonstrates how to:
- Connect to a remote agent service
- Use it just like a local agent
- Handle connection errors and retries

### 3. Mixed Local and Remote (`mixed_agents/`)

Illustrates how to:
- Create both local and remote agents
- Use them together as subagents
- Build a distributed agent system

### 4. Microservice Manager (`service_manager/`)

Shows how to:
- Manage multiple microservices
- Start/stop services programmatically
- Monitor service health

## Quick Start

1. **Start a microservice:**
```bash
cd basic_microservice
go run main.go
```

2. **Connect from another process:**
```bash
cd remote_client
go run main.go
```

3. **Test the mixed agents example:**
```bash
cd mixed_agents
go run main.go
```

## Architecture

```
┌─────────────────┐    gRPC    ┌─────────────────┐
│   Main Agent    │◄──────────►│ Remote Agent    │
│   (Local)       │            │ (Microservice)  │
│                 │            │                 │
│ ┌─────────────┐ │            │ ┌─────────────┐ │
│ │ Math Agent  │ │            │ │ Research    │ │
│ │ (Local)     │ │            │ │ Agent       │ │
│ └─────────────┘ │            │ │ (Local)     │ │
└─────────────────┘            │ └─────────────┘ │
                               └─────────────────┘
```

## Key Features

- **Transparent Usage**: Remote agents work exactly like local agents
- **Auto-Discovery**: Metadata is automatically fetched from remote services
- **Health Monitoring**: Built-in health checks and readiness probes
- **Error Handling**: Automatic retries and connection management
- **Security**: Support for authentication and TLS (in development)

## Running the Examples

Make sure you have the required dependencies:

```bash
go mod tidy
```

Then navigate to any example directory and run:

```bash
go run main.go
```

Some examples require multiple terminals to demonstrate client-server communication.