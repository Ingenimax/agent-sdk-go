package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/genai"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/sandbox"
)

// ShellTool is a tool that executes commands through the sandbox executor.
type ShellTool struct {
	executor sandbox.CommandExecutor
}

func (t *ShellTool) Name() string        { return "run_command" }
func (t *ShellTool) Description() string { return "Run a shell command in a sandboxed container. Only allowed commands can be executed." }
func (t *ShellTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"command": {Type: "string", Description: "The command to run (e.g., 'ls', 'cat', 'echo')", Required: true},
		"args":    {Type: "string", Description: "Space-separated arguments for the command", Required: false},
	}
}

func (t *ShellTool) Run(ctx context.Context, input string) (string, error) {
	return t.Execute(ctx, input)
}

func (t *ShellTool) Execute(ctx context.Context, args string) (string, error) {
	var params struct {
		Command string `json:"command"`
		Args    string `json:"args"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	// Split args string into slice
	var cmdArgs []string
	if params.Args != "" {
		// Simple split — good enough for demo
		for _, a := range splitArgs(params.Args) {
			cmdArgs = append(cmdArgs, a)
		}
	}

	cmd, err := t.executor.Command(ctx, params.Command, cmdArgs...)
	if err != nil {
		return fmt.Sprintf("Command denied: %v", err), nil
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Command failed: %v\nOutput: %s", err, string(output)), nil
	}

	return string(output), nil
}

func splitArgs(s string) []string {
	var args []string
	current := ""
	inQuote := false
	for _, c := range s {
		switch {
		case c == '"':
			inQuote = !inQuote
		case c == ' ' && !inQuote:
			if current != "" {
				args = append(args, current)
				current = ""
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		args = append(args, current)
	}
	return args
}

func main() {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("Set GEMINI_API_KEY environment variable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	logger := logging.New()

	// --- Step 1: Create sandbox executor ---
	fmt.Println("=== Creating Docker Sandbox ===")
	executor, err := sandbox.NewDockerExecutor(ctx, sandbox.Config{
		Enabled:         true,
		Image:           "alpine:3.19",
		AllowedCommands: []string{"echo", "ls", "cat", "uname", "whoami", "date", "hostname"},
		DeniedCommands:  []string{"rm", "dd", "mkfs", "chmod", "chown", "kill"},
		PoolSize:        1,
		Timeout:         10 * time.Second,
		NetworkMode:     "none",
		MemoryLimit:     "128m",
		CPULimit:        "0.5",
	}, logger)
	if err != nil {
		log.Fatalf("Failed to create sandbox: %v", err)
	}
	defer executor.Close(ctx)
	fmt.Println("Sandbox container ready!")
	fmt.Println()

	// --- Step 2: Create Gemini LLM client ---
	fmt.Println("=== Creating Gemini Agent ===")
	llm, err := gemini.NewClient(ctx,
		gemini.WithAPIKey(apiKey),
		gemini.WithBackend(genai.BackendGeminiAPI),
		gemini.WithModel(gemini.ModelGemini20Flash),
	)
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}
	fmt.Printf("LLM: %s (model: %s)\n", llm.Name(), llm.GetModel())

	// --- Step 3: Create agent with sandbox tool ---
	shellTool := &ShellTool{executor: executor}

	agentInstance, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithTools(shellTool),
		agent.WithSandbox(executor),
		agent.WithRequirePlanApproval(false),
		agent.WithSystemPrompt(`You are a system exploration agent running inside a secure sandbox container.
You have a run_command tool that executes commands inside a Docker container.
The container is isolated: no network, read-only filesystem, limited resources.

Only these commands are allowed: echo, ls, cat, uname, whoami, date, hostname.
Commands like rm, dd, chmod are blocked for security.

When the user asks you to explore, use the run_command tool directly. Do NOT create execution plans.`),
		agent.WithMaxIterations(10),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	fmt.Println("Agent ready!")
	fmt.Println()

	// --- Step 4: Run the agent ---
	fmt.Println("=== Agent Task: Explore the sandbox container ===")
	fmt.Println()

	result, err := agentInstance.Run(ctx,
		"Explore this sandbox container. Tell me: 1) What OS and architecture is it running? 2) What user are you? 3) What's the hostname? 4) List the files in /usr/bin/ (first 20 lines). 5) Try to run 'rm /tmp/test' to show it's blocked.")
	if err != nil {
		log.Fatalf("Agent error: %v", err)
	}

	fmt.Println("=== Agent Response ===")
	fmt.Println(result)
}
