package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
)

// CommandExecutor builds the process for a stdio MCP server, so the server can
// run somewhere other than directly on the host, such as in a container.
// Package sandbox provides a Docker implementation.
//
// Set it on StdioServerConfig.Executor or LazyMCPServerConfig.Executor, or
// give it to an agent with agent.WithMCPCommandExecutor. When it is nil, the
// server runs on the host.
type CommandExecutor interface {
	// Command returns an unstarted command that runs the program name with
	// args. env holds the server's own environment variables as "KEY=VALUE"
	// pairs. The executor resolves name itself: the host's PATH is not
	// searched, because the program may not exist on the host at all.
	Command(ctx context.Context, name string, args []string, env []string) (*exec.Cmd, error)
}

// executorKey identifies an executor in the lazy server cache, so servers
// started through different executors, or through none, are never shared.
func executorKey(e CommandExecutor) string {
	if e == nil {
		return ""
	}
	v := reflect.ValueOf(e)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Chan, reflect.Func, reflect.Slice, reflect.UnsafePointer:
		return fmt.Sprintf("%T@%x", e, v.Pointer())
	default:
		return fmt.Sprintf("%T:%v", e, e)
	}
}
