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
- `POST /api/v1/agent/delegate` - Delegate task to sub-agent (placeholder implementation)
- `GET /api/v1/memory` - Memory browser with pagination
- `GET /api/v1/memory/search` - Memory search functionality
- `GET /api/v1/tools` - Available tools list
- `WS /ws/chat` - WebSocket for real-time chat (placeholder - not yet implemented)

## Frontend Stack

### Technology
- **Next.js 16** with React 19 and TypeScript
- **shadcn/ui** components (built on Radix UI primitives)
- **Tailwind CSS** for styling
- **Static export** for embedding in Go binaries
- **Server-Sent Events (SSE)** for real-time streaming communication

### Key Components
```tsx
// Main layout with collapsible sidebar
<div className="flex h-screen">
  <Sidebar collapsible />
  <ChatArea className="flex-1" />
</div>

// Sub-agents section in sidebar (currently returns empty list)
// Note: Sub-agent functionality is implemented at API level but
// UI components are not fully developed yet
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

1. **Frontend**: Next.js app with shadcn/ui components (statically exported)
2. **Backend**: Extends existing HTTP server to serve UI files
3. **Single Binary**: No separate frontend deployment needed

```go
//go:embed all:ui-nextjs/out
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
- **Streaming Chat**: Real-time responses with Server-Sent Events (SSE) and typing indicators
- **Non-Streaming**: Traditional request/response mode toggle
- **Message History**: Persistent conversation history with search functionality
- **Character Count**: Real-time character counter for input
- **Auto-Resize**: Textarea automatically adjusts height based on content
- **Keyboard Shortcuts**: Enter to send, Shift+Enter for new line

### Agent Information
- **Model Details**: Current LLM model and settings extracted from agent
- **System Prompt**: View and understand agent behavior
- **Available Tools**: List of tools agent can use with descriptions
- **Memory Status**: Type and current state of memory system
- **Configuration Display**: Complete agent configuration in JSON format

### Memory Browser
- **Conversation History**: Browse past conversations with pagination (limit/offset)
- **Search**: Find specific messages or topics with query functionality
- **Agent Memory Integration**: Automatically retrieves from agent's memory system (Redis, etc.) when available
- **Fallback Storage**: In-memory conversation tracking when agent memory is unavailable
- **Metadata Support**: Timestamp and conversation ID tracking

### Sub-Agents (Limited Implementation)
- **API Endpoints**: Backend endpoints exist for sub-agent management
- **Delegation**: API endpoint for task delegation (placeholder implementation)
- **UI Components**: Frontend components exist but return empty lists currently
- **Future Enhancement**: Full sub-agent UI implementation planned

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

## Implementation Details

### Backend Implementation (Go)

**File Structure:**
- `ui_server.go`: Main UI server implementation with embedded file serving
- `HTTPServerWithUI`: Extends base HTTPServer with UI capabilities
- Embedded files: `//go:embed all:ui-nextjs/out` embeds static files at compile time

**Key Features:**
- **Memory Integration**: Automatically retrieves from agent's memory system when available
- **Tool Extraction**: Dynamically gets tool names from agent's tool registry
- **Model Detection**: Extracts model information from LLM interface
- **CORS Support**: Built-in CORS handling for API endpoints
- **Health Checks**: Debug endpoint to list embedded files
- **Organization Context**: Multi-tenancy support with default organization

**Memory System:**
- Primary: Retrieves from agent's memory interface (Redis, etc.)
- Fallback: In-memory conversation storage (last 1000 entries)
- Pagination: Supports limit/offset parameters
- Search: Text-based search through conversation history

### Frontend Implementation (Next.js)

**File Structure:**
- `app/`: Next.js 16 app directory with layout and pages
- `components/`: Reusable React components organized by feature
- `lib/`: API client and utility functions
- `types/`: TypeScript type definitions

**Key Components:**
- **MainLayout**: Root layout with responsive sidebar and header
- **ChatArea**: Chat interface with streaming and non-streaming support
- **Sidebar**: Agent information, tools, and memory browser
- **API Client**: TypeScript client with SSE streaming support

**SSE Streaming Implementation:**
```typescript
async *streamAgent(data: StreamRequest): AsyncGenerator<StreamEventData> {
  const response = await fetch(`/api/v1/agent/stream`, {
    method: 'POST',
    headers: { 'Accept': 'text/event-stream' },
    body: JSON.stringify(data),
  });

  const reader = response.body?.getReader();
  // Parse SSE events and yield data
}
```

## UI Development

The Next.js UI source code is located in `pkg/microservice/ui-nextjs/`. To modify the UI:

1. Navigate to the UI directory: `cd pkg/microservice/ui-nextjs/`
2. Install dependencies: `npm install`
3. Start development server: `npm run dev`
4. Build for production: `npm run build` (outputs to `out/` directory)
5. The Go application automatically embeds the `out/` directory

**Development Workflow:**
- Edit React components in `components/` and `app/` directories
- Modify API types in `types/agent.ts`
- Update API client in `lib/api.ts`
- Test with local Go server running UI server

**Build Process:**
- Next.js builds to static files in `out/` directory
- Go embed directive includes all files in binary
- Production deployment requires no separate frontend server

## Important: Rebuilding Embedded UI

⚠️ **The UI is embedded in the Go binary at compile time.** After making changes to the UI, you MUST:

1. **Build the UI assets:**
   ```bash
   cd pkg/microservice/ui-nextjs
   npm run build
   ```

2. **Rebuild your Go application** to embed the new UI:
   ```bash
   # For SDK development
   go build ./pkg/...

   # For your application using the SDK
   go build -o your-agent main.go
   ```

3. **Restart your agent** to see the changes

**Note:** Simply refreshing the browser will NOT show UI changes unless you rebuild both the UI and the Go binary. The UI is served from embedded files, not from the filesystem.

### Quick Build Script

For convenience, you can create a script to rebuild everything:

```bash
#!/bin/bash
# build-ui.sh

# Build the UI
cd pkg/microservice/ui-nextjs
npm run build

# Go back to project root
cd ../../..

# Rebuild Go packages
go build ./pkg/...

# Rebuild your application (adjust path as needed)
# go build -o my-agent ./cmd/my-app

echo "✅ UI and Go binary rebuilt. Restart your agent to see changes."
```