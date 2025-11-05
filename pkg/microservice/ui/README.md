# Agent UI Frontend

A modern, responsive web interface for the Agent SDK built with vanilla JavaScript, HTML, and CSS - no frameworks required.

## Overview

This UI provides a beautiful chat interface with:
- **Real-time streaming** responses via Server-Sent Events (SSE)
- **Collapsible sidebar** with agent information, tools, and settings
- **Dark/light theme** toggle with system preference detection
- **Responsive design** for desktop, tablet, and mobile
- **Memory browser** for conversation history
- **Tool visualization** showing when and how tools are used

## Files

- `index.html` - Main HTML structure with sidebar and chat layout
- `styles.css` - Modern CSS with CSS variables for theming
- `api.js` - API service layer for backend communication
- `app.js` - Main application logic and UI interactions

## Features

### Chat Interface
- Real-time streaming chat with typing indicators
- Message history with timestamps
- Tool call visualization
- Character count and input validation
- Auto-resizing textarea

### Sidebar Sections
- **Agent Info**: Name, model, description, system prompt
- **Tools**: List of available tools with descriptions
- **Memory**: Memory type, status, and history browser
- **Settings**: Theme toggle, streaming preferences

### API Integration
- Full REST API support for all backend endpoints
- Server-Sent Events (SSE) for real-time streaming
- Automatic error handling and reconnection
- Conversation management with unique IDs

### Responsive Design
- Desktop: Full sidebar + chat layout
- Tablet: Collapsible sidebar
- Mobile: Drawer-style sidebar with touch support

## Usage

The UI is automatically embedded in the Go binary and served at the root path when using `NewHTTPServerWithUI()`.

```go
server := microservice.NewHTTPServerWithUI(agent, port, nil)
server.Start()
// UI available at http://localhost:port/
```

## Browser Support

- Modern browsers with ES6+ support
- Server-Sent Events (SSE) support
- CSS Grid and Flexbox support

## Development

To modify the UI:

1. Edit the files in this directory
2. Rebuild your Go application
3. The updated UI will be embedded automatically

The UI uses vanilla JavaScript for maximum compatibility and minimal bundle size.