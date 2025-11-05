# Agent UI Interface

A modern web interface for interacting with agents. The UI is **automatically embedded** and served by your Go application.

## Overview

Simply enable the UI and get:
- **Beautiful chat interface** with collapsible sidebar
- **Real-time streaming** responses
- **Agent details** (model, tools, memory)
- **Sub-agents management** with delegation capabilities
- **Memory browser** to view conversation history

## Quick Start

Add **one line** to your existing agent code:

```go
package main

import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/microservice"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
    // Create your agent as usual
    llm := openai.NewClient("your-api-key")

    myAgent, err := agent.NewAgent(
        agent.WithLLM(llm),
        agent.WithName("MyAssistant"),
        agent.WithSystemPrompt("You are a helpful AI assistant"),
    )
    if err != nil {
        panic(err)
    }

    // ✨ Just replace this line:
    // server := microservice.NewHTTPServer(myAgent, 8080)

    // With this:
    server := microservice.NewHTTPServerWithUI(myAgent, 8080)

    server.Start()
    // UI automatically available at: http://localhost:8080/
}
```

**That's it!** The frontend is automatically served - no separate deployment needed.

## UI Layout

### Left Sidebar (Collapsible)
- **Agent Info Panel**
  - Agent name and description
  - Model information (GPT-4, Claude, etc.)
  - System prompt
  - Available tools
  - Memory type and status

- **Sub-Agents**
  - List of available sub-agents with details:
    - Name and description
    - Specialized capabilities
    - Model/LLM being used
    - Tools available to each sub-agent
  - Quick switch between agents
  - Active/inactive status indicators
  - Delegation history and interactions
  - Sub-agent performance metrics

- **Memory Browser**
  - Conversation history
  - Search functionality
  - Message filtering
  - Export options

- **Settings**
  - Theme toggle (light/dark)
  - Streaming preferences
  - API configuration

### Main Chat Area
- **Chat Interface**
  - Real-time streaming chat
  - Non-streaming mode toggle
  - Message history
  - Tool call visualization
  - Copy/share functionality

- **Response Modes**
  - **Streaming**: Real-time response as it's generated
  - **Single**: Wait for complete response

## API Endpoints

The UI communicates with these endpoints:

### Existing Endpoints
- `POST /api/v1/agent/run` - Non-streaming chat
- `POST /api/v1/agent/stream` - SSE streaming chat
- `GET /api/v1/agent/metadata` - Agent information
- `GET /health` - Health check

### New UI-Specific Endpoints
- `GET /api/v1/agent/config` - Detailed agent configuration
- `GET /api/v1/agent/subagents` - List all sub-agents with details
- `GET /api/v1/agent/subagents/{id}` - Get specific sub-agent info
- `POST /api/v1/agent/delegate` - Delegate task to sub-agent
- `GET /api/v1/memory` - Memory browser
- `GET /api/v1/memory/search` - Memory search
- `GET /api/v1/tools` - Available tools list
- `WS /ws/chat` - WebSocket for real-time chat

## Frontend Stack

### Technology
- **React 18** with TypeScript
- **shadcn/ui** components
- **Tailwind CSS** for styling
- **Vite** for development and building
- **WebSocket/SSE** for real-time communication

### Key Components
```tsx
// Main layout with collapsible sidebar
<div className="flex h-screen">
  <Sidebar collapsible />
  <ChatArea className="flex-1" />
</div>

// Sub-agents section in sidebar
<Collapsible>
  <CollapsibleTrigger>
    <h3>Sub-Agents ({subAgents.length})</h3>
  </CollapsibleTrigger>
  <CollapsibleContent>
    {subAgents.map(agent => (
      <Card key={agent.id} className="mb-2">
        <CardHeader className="p-3">
          <div className="flex justify-between">
            <span>{agent.name}</span>
            <Badge>{agent.status}</Badge>
          </div>
        </CardHeader>
        <CardContent className="p-3">
          <p className="text-sm">{agent.description}</p>
          <div className="flex gap-1 mt-2">
            <Badge variant="outline">{agent.model}</Badge>
            <Badge variant="outline">{agent.tools.length} tools</Badge>
          </div>
        </CardContent>
      </Card>
    ))}
  </CollapsibleContent>
</Collapsible>

// shadcn/ui components used:
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
```

## How It Works

The UI is **automatically embedded** in your Go binary:

1. **Frontend**: React app with shadcn/ui components (pre-built)
2. **Backend**: Extends existing HTTP server to serve UI files
3. **Single Binary**: No separate frontend deployment needed

```go
//go:embed ui/dist/*
var uiFiles embed.FS

// UI automatically served at root path
mux.Handle("/", http.FileServer(http.FS(uiFiles)))
```

## Configuration Options

### Agent Configuration
```go
// Enable UI with default settings
server := microservice.NewHTTPServerWithUI(agent, port, nil)

// Custom configuration
uiConfig := &microservice.UIConfig{
    Enabled:     true,
    DefaultPath: "/",           // Serve UI at root
    DevMode:     false,         // Production mode
    Theme:       "light",       // Default theme
    Features: microservice.UIFeatures{
        Chat:         true,      // Enable chat interface
        Memory:       true,      // Enable memory browser
        AgentInfo:    true,      // Show agent details
        Settings:     true,      // Show settings panel
    },
}
```

### Environment Variables
```bash
AGENT_UI_ENABLED=true          # Enable/disable UI
AGENT_UI_PATH=/                # UI path (default: /)
AGENT_UI_DEV_MODE=false        # Development mode
AGENT_UI_THEME=light           # Default theme
```

## Features

### Chat Interface
- **Streaming Chat**: Real-time responses with typing indicators
- **Non-Streaming**: Traditional request/response
- **Message History**: Persistent conversation history
- **Tool Visualization**: See when and how tools are used
- **Export**: Save conversations as markdown/JSON

### Agent Information
- **Model Details**: Current LLM model and settings
- **System Prompt**: View and understand agent behavior
- **Available Tools**: List of tools agent can use
- **Memory Status**: Type and current state of memory
- **Sub-Agents**: View and interact with specialized sub-agents
  - Each sub-agent shows its own model, tools, and capabilities
  - Quick delegation to sub-agents for specific tasks
  - Monitor sub-agent activity and performance

### Memory Browser
- **Conversation History**: Browse past conversations
- **Search**: Find specific messages or topics
- **Filtering**: Filter by role, date, or content type
- **Export**: Export memory data

### Responsive Design
- **Desktop**: Full sidebar + chat layout
- **Tablet**: Collapsible sidebar
- **Mobile**: Drawer-style sidebar, optimized chat

## Deployment

**Simple!** Just build and run your Go application:

```bash
go build -o my-agent ./cmd/my-app
./my-agent

# UI automatically available at http://localhost:8080/
```

The UI is embedded in the binary - no separate deployment needed!

## Benefits

1. **Simple Integration**: Minimal configuration required
2. **Professional UI**: Modern, responsive design with shadcn/ui
3. **Real-time Features**: Streaming responses and live updates
4. **Single Binary**: No separate deployment needed
5. **Developer Friendly**: Hot reload in development
6. **Extensible**: Easy to add new features and components

## Examples

See `examples/ui/` directory for complete implementation examples:
- Basic agent with UI
- Custom configuration
- Multi-agent setup
- Development workflow