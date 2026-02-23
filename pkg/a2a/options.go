package a2a

import (
	"net/http"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
)

// ServerOption configures an A2A server.
type ServerOption func(*Server)

// WithAddress sets the listen address for the A2A server.
func WithAddress(addr string) ServerOption {
	return func(s *Server) {
		s.addr = addr
	}
}

// WithBasePath sets the JSON-RPC endpoint base path.
// Defaults to "/".
func WithBasePath(path string) ServerOption {
	return func(s *Server) {
		s.basePath = path
	}
}

// WithServerLogger sets a logger for the A2A server.
func WithServerLogger(logger logging.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithShutdownTimeout sets the graceful shutdown timeout for the A2A server.
// Defaults to 30 seconds.
func WithShutdownTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.shutdownTimeout = d
	}
}

// WithMiddleware adds an HTTP middleware to the A2A server.
// Middleware is applied in the order provided, wrapping the base handler.
// Use this for authentication, rate limiting, CORS, logging, etc.
func WithMiddleware(mw func(http.Handler) http.Handler) ServerOption {
	return func(s *Server) {
		s.middlewares = append(s.middlewares, mw)
	}
}

// ClientOption configures an A2A client.
type ClientOption func(*Client)

// WithClientLogger sets a logger for the A2A client.
func WithClientLogger(logger logging.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithTimeout sets the HTTP client timeout for the A2A client.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = d
	}
}

// CardOption configures an AgentCard builder.
type CardOption func(*CardBuilder)

// WithVersion sets the agent version on the card.
func WithVersion(version string) CardOption {
	return func(b *CardBuilder) {
		b.version = version
	}
}

// WithProviderInfo sets the provider organization info on the card.
func WithProviderInfo(org, url string) CardOption {
	return func(b *CardBuilder) {
		b.providerOrg = org
		b.providerURL = url
	}
}

// WithDocumentationURL sets the documentation URL on the card.
func WithDocumentationURL(url string) CardOption {
	return func(b *CardBuilder) {
		b.documentationURL = url
	}
}

// WithStreaming enables or disables streaming capability on the card.
func WithStreaming(enabled bool) CardOption {
	return func(b *CardBuilder) {
		b.streaming = enabled
	}
}

// WithInputModes sets the default accepted input MIME types.
func WithInputModes(modes ...string) CardOption {
	return func(b *CardBuilder) {
		b.inputModes = modes
	}
}

// WithOutputModes sets the default accepted output MIME types.
func WithOutputModes(modes ...string) CardOption {
	return func(b *CardBuilder) {
		b.outputModes = modes
	}
}

// WithBearerToken sets a static bearer token for authentication on the A2A client.
// The token is injected into every outgoing request as an Authorization header.
func WithBearerToken(token string) ClientOption {
	return func(c *Client) {
		c.bearerToken = token
	}
}

// SendOption configures individual SendMessage / SendMessageStream calls.
type SendOption func(*sendConfig)

// sendConfig holds per-call options for send operations.
type sendConfig struct {
	contextID string
	taskID    string
}

// WithContextID sets the context ID for multi-turn conversations.
// Messages sharing a context ID are grouped into the same interaction thread.
func WithContextID(id string) SendOption {
	return func(c *sendConfig) {
		c.contextID = id
	}
}

// WithTaskID continues an existing task by referencing its ID.
func WithTaskID(id string) SendOption {
	return func(c *sendConfig) {
		c.taskID = id
	}
}
