// Package a2a provides Google A2A (Agent-to-Agent) protocol support for agent-sdk-go.
//
// It enables cross-framework agent interoperability by implementing the A2A specification,
// allowing agents built with agent-sdk-go to communicate with agents built on any
// A2A-compliant framework (Google ADK, LangChain, CrewAI, etc.).
//
// The package provides three main components:
//
//   - Server: Exposes an agent-sdk-go agent as an A2A-compliant HTTP server, complete with
//     agent card discovery, JSON-RPC message handling, and streaming support.
//
//   - Client: Discovers and communicates with remote A2A agents, supporting both synchronous
//     and streaming message patterns, multi-turn conversations, and task management.
//
//   - RemoteAgentTool: Wraps a remote A2A agent as an interfaces.Tool, enabling seamless
//     integration of remote agents into local agent tool chains.
//
// Server usage:
//
//	card := a2a.NewCardBuilder("My Agent", "description", "http://localhost:9100/",
//	    a2a.WithStreaming(true),
//	).Build()
//
//	srv := a2a.NewServer(agent, card,
//	    a2a.WithAddress(":9100"),
//	    a2a.WithMiddleware(authMiddleware),
//	)
//	srv.Start(ctx)
//
// Client usage:
//
//	client, err := a2a.NewClient(ctx, "http://remote-agent:9100",
//	    a2a.WithBearerToken("token"),
//	)
//	result, err := client.SendMessage(ctx, "hello",
//	    a2a.WithContextID("conversation-1"),
//	)
package a2a
