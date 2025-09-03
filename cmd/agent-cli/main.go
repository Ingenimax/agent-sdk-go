package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vertex"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/mcp"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/github"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/websearch"
	"github.com/Ingenimax/agent-sdk-go/pkg/tracing"
)

const (
	version = "0.0.39"
	banner  = `
╔══════════════════════════════════════════════════════════════════════════════╗
║                           Agent SDK CLI Tool                                 ║
║                         Headless AI Agent Runner                             ║
║                              Version %s                                   ║
╚══════════════════════════════════════════════════════════════════════════════╝
`
)

// Global logger instance
var logger = logging.New()

type CLIConfig struct {
	Provider       string            `json:"provider"`        // openai, anthropic, vertex, ollama, vllm
	Model          string            `json:"model"`           // model name
	SystemPrompt   string            `json:"system_prompt"`   // default system prompt
	Temperature    float64           `json:"temperature"`     // temperature setting
	MaxIterations  int               `json:"max_iterations"`  // max tool iterations
	OrgID          string            `json:"org_id"`          // organization ID
	ConversationID string            `json:"conversation_id"` // conversation ID
	EnableTracing  bool              `json:"enable_tracing"`  // enable tracing
	EnableMemory   bool              `json:"enable_memory"`   // enable memory
	EnableTools    bool              `json:"enable_tools"`    // enable tools
	MCPServers     []MCPServerConfig `json:"mcp_servers"`     // MCP server configs
	Variables      map[string]string `json:"variables"`       // template variables
}

type MCPServerConfig struct {
	Name    string            `json:"name"`    // server name
	Type    string            `json:"type"`    // http or stdio
	URL     string            `json:"url"`     // for http servers
	Command string            `json:"command"` // for stdio servers
	Args    []string          `json:"args"`    // for stdio servers
	Env     map[string]string `json:"env"`     // environment variables
}

// MCPServersConfig represents the structure for loading MCP servers from JSON config
type MCPServersConfig struct {
	MCPServers map[string]MCPServerDefinition `json:"mcpServers"`
}

// MCPServerDefinition represents a single MCP server definition in JSON config
type MCPServerDefinition struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url,omitempty"` // for HTTP servers
}

func main() {
	// Load .env file if it exists
	loadEnvFile()

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	// Check for direct execution flags first
	if hasDirectExecutionFlags() {
		executeDirectPrompt()
		return
	}

	command := os.Args[1]

	switch command {
	case "version", "--version", "-v":
		fmt.Printf("Agent SDK CLI v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	case "init":
		initConfig()
	case "config":
		manageConfig()
	case "run":
		runAgent()
	case "task":
		executeTask()
	case "chat":
		startInteractiveChat()
	case "generate":
		generateConfigs()
	case "list":
		listResources()
	case "mcp":
		manageMCP()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

// DirectExecutionConfig holds configuration for direct prompt execution
type DirectExecutionConfig struct {
	Prompt                     string
	MCPConfigFile              string
	AllowedTools               []string
	DangerouslySkipPermissions bool
}

func hasDirectExecutionFlags() bool {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "--prompt") {
			return true
		}
	}
	return false
}

func parseDirectExecutionFlags() (*DirectExecutionConfig, error) {
	config := &DirectExecutionConfig{}

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]

		if strings.HasPrefix(arg, "--prompt=") {
			config.Prompt = strings.TrimPrefix(arg, "--prompt=")
		} else if arg == "--prompt" && i+1 < len(os.Args) {
			config.Prompt = os.Args[i+1]
			i++ // skip next arg
		} else if strings.HasPrefix(arg, "--mcp-config=") {
			config.MCPConfigFile = strings.TrimPrefix(arg, "--mcp-config=")
		} else if arg == "--mcp-config" && i+1 < len(os.Args) {
			config.MCPConfigFile = os.Args[i+1]
			i++ // skip next arg
		} else if strings.HasPrefix(arg, "--allowedTools=") {
			toolsStr := strings.TrimPrefix(arg, "--allowedTools=")
			config.AllowedTools = strings.Split(toolsStr, ",")
		} else if arg == "--allowedTools" && i+1 < len(os.Args) {
			config.AllowedTools = strings.Split(os.Args[i+1], ",")
			i++ // skip next arg
		} else if arg == "--dangerously-skip-permissions" {
			config.DangerouslySkipPermissions = true
		}
	}

	if config.Prompt == "" {
		return nil, fmt.Errorf("--prompt is required for direct execution")
	}

	return config, nil
}

func executeDirectPrompt() {
	ctx := context.Background()
	logger.Info(ctx, "Starting direct prompt execution mode", nil)

	// Parse direct execution flags
	directConfig, err := parseDirectExecutionFlags()
	if err != nil {
		logger.Error(ctx, "Failed to parse direct execution flags", map[string]interface{}{
			"error": err.Error(),
		})
		printDirectUsage()
		return
	}

	logger.Info(ctx, "Direct prompt execution configured", map[string]interface{}{
		"prompt":                       directConfig.Prompt,
		"mcp_config_file":              directConfig.MCPConfigFile,
		"allowed_tools_count":          len(directConfig.AllowedTools),
		"dangerously_skip_permissions": directConfig.DangerouslySkipPermissions,
	})

	// Load MCP servers from config file if provided
	var mcpServers []MCPServerConfig
	if directConfig.MCPConfigFile != "" {
		logger.Info(ctx, "Loading MCP configuration", map[string]interface{}{
			"config_file": directConfig.MCPConfigFile,
		})
		mcpServers, err = loadMCPServersFromFile(directConfig.MCPConfigFile)
		if err != nil {
			logger.Error(ctx, "Failed to load MCP config", map[string]interface{}{
				"config_file": directConfig.MCPConfigFile,
				"error":       err.Error(),
			})
			return
		}
		logger.Info(ctx, "MCP configuration loaded successfully", map[string]interface{}{
			"server_count": len(mcpServers),
		})
	}

	// Log allowed tools if specified
	if len(directConfig.AllowedTools) > 0 {
		logger.Info(ctx, "Tool filtering enabled", map[string]interface{}{
			"allowed_tools": directConfig.AllowedTools,
		})
	}

	// Log warning if skipping permissions
	if directConfig.DangerouslySkipPermissions {
		logger.Warn(ctx, "Permission checks disabled - use with caution", nil)
	}

	// Execute the agent with the direct configuration
	err = runDirectAgent(directConfig, mcpServers)
	if err != nil {
		logger.Error(ctx, "Direct execution failed", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
}

func loadMCPServersFromFile(filePath string) ([]MCPServerConfig, error) {
	// Read JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Parse JSON
	var mcpConfig MCPServersConfig
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Convert to MCPServerConfig slice
	var servers []MCPServerConfig
	for serverName, serverDef := range mcpConfig.MCPServers {
		// Determine server type
		serverType := "stdio" // default
		if serverDef.URL != "" {
			serverType = "http"
		}

		server := MCPServerConfig{
			Name:    serverName,
			Type:    serverType,
			Command: serverDef.Command,
			Args:    serverDef.Args,
			URL:     serverDef.URL,
			Env:     serverDef.Env,
		}

		// Initialize Env map if nil
		if server.Env == nil {
			server.Env = make(map[string]string)
		}

		servers = append(servers, server)
	}

	return servers, nil
}

func runDirectAgent(directConfig *DirectExecutionConfig, mcpServers []MCPServerConfig) error {
	ctx := context.Background()

	// Load base configuration
	config := loadConfig()
	logger.Debug(ctx, "Base configuration loaded", map[string]interface{}{
		"provider": config.Provider,
		"model":    config.Model,
	})

	// Create LLM client
	llmClient := createLLM(config)
	if llmClient == nil {
		return fmt.Errorf("failed to create LLM client")
	}
	logger.Info(ctx, "LLM client created successfully", map[string]interface{}{
		"provider": config.Provider,
		"model":    config.Model,
	})

	// Create tools
	var tools []interfaces.Tool

	// Add regular tools if no specific tools are allowed or if they're in the allowed list
	if len(directConfig.AllowedTools) == 0 || containsAnyTool(directConfig.AllowedTools, []string{"websearch", "github"}) {
		regularTools := createTools()
		tools = append(tools, regularTools...)
	}

	// Create lazy MCP configurations (servers will be initialized on demand)
	var lazyMCPConfigs []agent.LazyMCPConfig
	if len(mcpServers) > 0 {
		lazyMCPConfigs = createLazyMCPConfigs(mcpServers, directConfig.AllowedTools)
		logger.Info(ctx, "Lazy MCP configurations created", map[string]interface{}{
			"server_count":              len(lazyMCPConfigs),
			"filtered_by_allowed_tools": len(directConfig.AllowedTools) > 0,
		})
	}

	logger.Info(ctx, "Tool configuration completed", map[string]interface{}{
		"regular_tools_count": len(tools),
		"mcp_servers_count":   len(lazyMCPConfigs),
	})

	// Create agent options following the example pattern
	var agentOptions []agent.Option
	agentOptions = append(agentOptions, agent.WithLLM(llmClient))

	// Set plan approval requirement based on dangerously-skip-permissions flag
	agentOptions = append(agentOptions, agent.WithRequirePlanApproval(!directConfig.DangerouslySkipPermissions))

	// Add regular tools if any
	if len(tools) > 0 {
		agentOptions = append(agentOptions, agent.WithTools(tools...))
	}

	// Add lazy MCP configurations to the agent
	if len(lazyMCPConfigs) > 0 {
		agentOptions = append(agentOptions, agent.WithLazyMCPConfigs(lazyMCPConfigs))
	}

	// Add tracer if configured
	if tracer := createTracer(); tracer != nil {
		agentOptions = append(agentOptions, agent.WithTracer(tracer))
	}

	// Create and run agent
	agentInstance, err := agent.NewAgent(agentOptions...)
	if err != nil {
		logger.Error(ctx, "Failed to create agent", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to create agent: %v", err)
	}

	logger.Info(ctx, "Agent created successfully, executing prompt", map[string]interface{}{
		"prompt_length": len(directConfig.Prompt),
	})

	// Execute the prompt
	response, err := agentInstance.Run(ctx, directConfig.Prompt)
	if err != nil {
		logger.Error(ctx, "Agent execution failed", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("agent execution failed: %v", err)
	}

	logger.Info(ctx, "Agent execution completed successfully", map[string]interface{}{
		"response_length": len(response),
	})

	// Still print the response to stdout for user visibility
	fmt.Println()
	fmt.Println("📋 Response:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(response)

	return nil
}

func containsAnyTool(allowedTools, toolNames []string) bool {
	for _, allowed := range allowedTools {
		for _, toolName := range toolNames {
			if strings.Contains(allowed, toolName) {
				return true
			}
		}
	}
	return false
}



func printDirectUsage() {
	fmt.Println()
	fmt.Println("Direct Execution Mode:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("USAGE:")
	fmt.Println("    agent-cli --prompt \"<your prompt>\" [options]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("    --prompt <text>                    The prompt to execute")
	fmt.Println("    --mcp-config <file>               JSON file with MCP server configuration")
	fmt.Println("    --allowedTools <tool1,tool2>      Comma-separated list of allowed tools")
	fmt.Println("    --dangerously-skip-permissions    Skip permission checks (use with caution)")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("    # Simple prompt execution")
	fmt.Println("    agent-cli --prompt \"What is the weather today?\"")
	fmt.Println()
	fmt.Println("    # With MCP server and specific tools")
	fmt.Println("    agent-cli --prompt \"List my EC2 instances\" \\")
	fmt.Println("      --mcp-config ./aws_api_server.json \\")
	fmt.Println("      --allowedTools \"mcp__aws__suggest_aws_commands,mcp__aws__call_aws\" \\")
	fmt.Println("      --dangerously-skip-permissions")
	fmt.Println()
}

func printUsage() {
	fmt.Printf(banner, version)
	fmt.Println(`USAGE:
    agent-cli <command> [options]
    agent-cli --prompt "<prompt>" [options]  # Direct execution mode

COMMANDS:
    init                Initialize CLI configuration
    config              Manage configuration settings
    run                 Run agent with a single prompt
    task                Execute predefined tasks from YAML
    chat                Start interactive chat session
    generate            Generate agent/task configurations
    list                List available resources
    mcp                 Manage MCP servers (add, list, remove)
    version             Show version information
    help                Show this help message

EXAMPLES:
    # Initialize configuration
    agent-cli init

    # Run a simple agent
    agent-cli run "What's the weather in San Francisco?"

    # Direct execution with prompt
    agent-cli --prompt "What is the weather today?"

    # Direct execution with MCP server
    agent-cli --prompt "List my EC2 instances" \
      --mcp-config ./aws_api_server.json \
      --allowedTools "mcp__aws__suggest_aws_commands,mcp__aws__call_aws" \
      --dangerously-skip-permissions

    # Execute a task from YAML config
    agent-cli task --config agents.yaml --task research_task --topic "AI"

    # Start interactive chat
    agent-cli chat

    # Generate configurations from system prompt
    agent-cli generate --prompt "You are a travel advisor"

    # Manage MCP servers
    agent-cli mcp add --type http --url http://localhost:8083/mcp
    agent-cli mcp list

For more detailed help on each command, use:
    agent-cli <command> --help`)
}

func initConfig() {
	fmt.Println("🚀 Initializing Agent SDK CLI configuration...")

	configDir := getConfigDir()
	if err := os.MkdirAll(configDir, 0750); err != nil {
		ctx := context.Background()
		logger.Error(ctx, "Failed to create config directory", map[string]interface{}{
			"config_dir": configDir,
			"error":      err.Error(),
		})
		os.Exit(1)
	}

	configFile := filepath.Join(configDir, "config.json")

	// Check if config already exists
	if _, err := os.Stat(configFile); err == nil {
		fmt.Print("Configuration already exists. Overwrite? (y/N): ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Println("Configuration initialization cancelled.")
			return
		}
	}

	// Interactive configuration setup
	config := &CLIConfig{
		Provider:       "openai",
		Model:          "gpt-4o-mini",
		SystemPrompt:   "You are a helpful AI assistant.",
		Temperature:    0.7,
		MaxIterations:  2,
		OrgID:          "default-org",
		ConversationID: "cli-session",
		EnableTracing:  false,
		EnableMemory:   true,
		EnableTools:    true,
		Variables:      make(map[string]string),
	}

	reader := bufio.NewReader(os.Stdin)

	// Provider selection
	fmt.Print("Select LLM provider (openai/anthropic/vertex/ollama/vllm) [openai]: ")
	if input, _ := reader.ReadString('\n'); strings.TrimSpace(input) != "" {
		config.Provider = strings.TrimSpace(input)
	}

	// Model selection
	defaultModel := getDefaultModel(config.Provider)
	fmt.Printf("Model name [%s]: ", defaultModel)
	if input, _ := reader.ReadString('\n'); strings.TrimSpace(input) != "" {
		config.Model = strings.TrimSpace(input)
	} else {
		config.Model = defaultModel
	}

	// System prompt
	fmt.Print("Default system prompt [You are a helpful AI assistant.]: ")
	if input, _ := reader.ReadString('\n'); strings.TrimSpace(input) != "" {
		config.SystemPrompt = strings.TrimSpace(input)
	}

	// Save configuration
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		ctx := context.Background()
		logger.Error(ctx, "Failed to marshal configuration", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if err := os.WriteFile(configFile, configData, 0600); err != nil {
		ctx := context.Background()
		logger.Error(ctx, "Failed to write configuration", map[string]interface{}{
			"config_file": configFile,
			"error":       err.Error(),
		})
		os.Exit(1)
	}

	fmt.Printf("✅ Configuration saved to: %s\n", configFile)
	fmt.Println("\n📝 Next steps:")
	fmt.Println("1. Set your API keys as environment variables:")

	switch config.Provider {
	case "openai":
		fmt.Println("   export OPENAI_API_KEY=your_api_key_here")
	case "anthropic":
		fmt.Println("   export ANTHROPIC_API_KEY=your_api_key_here")
	case "vertex":
		fmt.Println("   export GOOGLE_APPLICATION_CREDENTIALS=path_to_service_account.json")
	case "ollama":
		fmt.Println("   Make sure Ollama is running on localhost:11434")
	case "vllm":
		fmt.Println("   Make sure vLLM server is running on localhost:8000")
	}

	fmt.Println("2. Run: agent-cli run \"Hello, world!\"")
}

func manageConfig() {
	if len(os.Args) < 3 {
		showConfig()
		return
	}

	subcommand := os.Args[2]
	switch subcommand {
	case "show":
		showConfig()
	case "set":
		setConfigValue()
	case "reset":
		resetConfig()
	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcommand)
		fmt.Println("Available: show, set, reset")
	}
}

func showConfig() {
	config := loadConfig()
	configData, _ := json.MarshalIndent(config, "", "  ")
	fmt.Println("Current configuration:")
	fmt.Println(string(configData))
}

func setConfigValue() {
	if len(os.Args) < 5 {
		fmt.Println("Usage: agent-cli config set <key> <value>")
		return
	}

	key := os.Args[3]
	value := os.Args[4]

	config := loadConfig()

	switch key {
	case "provider":
		config.Provider = value
	case "model":
		config.Model = value
	case "system_prompt":
		config.SystemPrompt = value
	case "org_id":
		config.OrgID = value
	case "conversation_id":
		config.ConversationID = value
	default:
		fmt.Printf("Unknown config key: %s\n", key)
		return
	}

	saveConfig(config)
	fmt.Printf("✅ Set %s = %s\n", key, value)
}

func resetConfig() {
	configFile := filepath.Join(getConfigDir(), "config.json")
	if err := os.Remove(configFile); err != nil {
		log.Printf("Failed to remove config file: %v", err)
	}
	fmt.Println("✅ Configuration reset. Run 'agent-cli init' to reconfigure.")
}

func runAgent() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agent-cli run \"<prompt>\"")
		fmt.Println("Example: agent-cli run \"What's the weather in San Francisco?\"")
		return
	}

	prompt := strings.Join(os.Args[2:], " ")

	// Remove quotes if present
	if strings.HasPrefix(prompt, "\"") && strings.HasSuffix(prompt, "\"") {
		prompt = prompt[1 : len(prompt)-1]
	}

	fmt.Printf("🤖 Running agent with prompt: %s\n", prompt)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	config := loadConfig()
	agent := createAgent(config)

	ctx := createContext(config)

	response, err := agent.Run(ctx, prompt)
	if err != nil {
		log.Fatalf("❌ Agent execution failed: %v", err)
	}

	fmt.Println("\n📝 Response:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(response)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func executeTask() {
	var agentConfigPath, taskConfigPath, taskName, topic string
	var variables = make(map[string]string)

	// Parse command line arguments
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case strings.HasPrefix(arg, "--agent-config="):
			agentConfigPath = strings.TrimPrefix(arg, "--agent-config=")
		case strings.HasPrefix(arg, "--task-config="):
			taskConfigPath = strings.TrimPrefix(arg, "--task-config=")
		case strings.HasPrefix(arg, "--task="):
			taskName = strings.TrimPrefix(arg, "--task=")
		case strings.HasPrefix(arg, "--topic="):
			topic = strings.TrimPrefix(arg, "--topic=")
		case strings.HasPrefix(arg, "--var="):
			varPair := strings.TrimPrefix(arg, "--var=")
			parts := strings.SplitN(varPair, "=", 2)
			if len(parts) == 2 {
				variables[parts[0]] = parts[1]
			}
		}
	}

	if agentConfigPath == "" || taskConfigPath == "" || taskName == "" {
		fmt.Println("Usage: agent-cli task --agent-config=<path> --task-config=<path> --task=<name> [--topic=<topic>] [--var=key=value]")
		fmt.Println("Example: agent-cli task --agent-config=agents.yaml --task-config=tasks.yaml --task=research_task --topic=\"AI\"")
		return
	}

	// Add topic to variables if provided
	if topic != "" {
		variables["topic"] = topic
	}

	fmt.Printf("🎯 Executing task: %s\n", taskName)
	fmt.Printf("📁 Agent config: %s\n", agentConfigPath)
	fmt.Printf("📁 Task config: %s\n", taskConfigPath)
	if len(variables) > 0 {
		fmt.Printf("🔧 Variables: %+v\n", variables)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Load configurations
	agentConfigs, err := agent.LoadAgentConfigsFromFile(agentConfigPath)
	if err != nil {
		log.Fatalf("❌ Failed to load agent configurations: %v", err)
	}

	taskConfigs, err := agent.LoadTaskConfigsFromFile(taskConfigPath)
	if err != nil {
		log.Fatalf("❌ Failed to load task configurations: %v", err)
	}

	// Create LLM client
	config := loadConfig()
	llm := createLLM(config)

	// Create agent for task
	taskAgent, err := agent.CreateAgentForTask(taskName, agentConfigs, taskConfigs, variables, agent.WithLLM(llm))
	if err != nil {
		log.Fatalf("❌ Failed to create agent for task: %v", err)
	}

	// Execute task
	ctx := createContext(config)
	result, err := taskAgent.ExecuteTaskFromConfig(ctx, taskName, taskConfigs, variables)
	if err != nil {
		log.Fatalf("❌ Failed to execute task: %v", err)
	}

	fmt.Println("\n📝 Task Result:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(result)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if output file was created
	taskConfig := taskConfigs[taskName]
	if taskConfig.OutputFile != "" {
		outputPath := taskConfig.OutputFile
		for key, value := range variables {
			placeholder := fmt.Sprintf("{%s}", key)
			outputPath = strings.ReplaceAll(outputPath, placeholder, value)
		}
		fmt.Printf("💾 Output saved to: %s\n", outputPath)
	}
}

func startInteractiveChat() {
	fmt.Printf(banner, version)
	fmt.Println("🗨️  Interactive Chat Mode")
	fmt.Println("Type 'exit', 'quit', or 'bye' to end the session")
	fmt.Println("Type 'help' for available commands")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	config := loadConfig()
	agent := createAgent(config)
	ctx := createContext(config)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\n🤖 You: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle special commands
		switch strings.ToLower(input) {
		case "exit", "quit", "bye":
			fmt.Println("👋 Goodbye!")
			return
		case "help":
			printChatHelp()
			continue
		case "clear":
			// Clear conversation memory
			fmt.Println("🧹 Conversation cleared")
			continue
		case "config":
			showConfig()
			continue
		}

		fmt.Print("🤖 Assistant: ")

		response, err := agent.Run(ctx, input)
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		fmt.Println(response)
	}
}

func printChatHelp() {
	fmt.Println(`Available commands:
  help     - Show this help message
  clear    - Clear conversation history
  config   - Show current configuration
  exit     - Exit chat mode
  quit     - Exit chat mode
  bye      - Exit chat mode

Just type your message to chat with the agent!`)
}

func manageMCP() {
	if len(os.Args) < 3 {
		printMCPUsage()
		return
	}

	subcommand := os.Args[2]
	switch subcommand {
	case "add":
		addMCPServer()
	case "list":
		listMCPServers()
	case "remove", "rm":
		removeMCPServer()
	case "test":
		testMCPServer()
	case "import":
		importMCPServers()
	case "export":
		exportMCPServers()
	default:
		fmt.Printf("Unknown MCP subcommand: %s\n", subcommand)
		printMCPUsage()
	}
}

func printMCPUsage() {
	fmt.Println("MCP Server Management")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("USAGE:")
	fmt.Println("    agent-cli mcp <subcommand> [options]")
	fmt.Println()
	fmt.Println("SUBCOMMANDS:")
	fmt.Println("    add     Add a new MCP server")
	fmt.Println("    list    List configured MCP servers")
	fmt.Println("    remove  Remove an MCP server")
	fmt.Println("    test    Test connection to an MCP server")
	fmt.Println("    import  Import MCP servers from JSON config file")
	fmt.Println("    export  Export MCP servers to JSON config file")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("    # Add HTTP MCP server")
	fmt.Println("    agent-cli mcp add --type http --url http://localhost:8083/mcp --name my-server")
	fmt.Println()
	fmt.Println("    # Add stdio MCP server")
	fmt.Println("    agent-cli mcp add --type stdio --command python --args \"-m,mcp_server\" --name python-server")
	fmt.Println()
	fmt.Println("    # List all MCP servers")
	fmt.Println("    agent-cli mcp list")
	fmt.Println()
	fmt.Println("    # Remove MCP server")
	fmt.Println("    agent-cli mcp remove --name my-server")
	fmt.Println()
	fmt.Println("    # Test MCP server connection")
	fmt.Println("    agent-cli mcp test --name my-server")
	fmt.Println()
	fmt.Println("    # Import MCP servers from JSON config")
	fmt.Println("    agent-cli mcp import --file mcp-config.json")
	fmt.Println()
	fmt.Println("    # Export MCP servers to JSON config")
	fmt.Println("    agent-cli mcp export --file mcp-config.json")
}

func addMCPServer() {
	var serverType, url, command, name string
	var args []string

	// Parse command line arguments
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case strings.HasPrefix(arg, "--type="):
			serverType = strings.TrimPrefix(arg, "--type=")
		case strings.HasPrefix(arg, "--url="):
			url = strings.TrimPrefix(arg, "--url=")
		case strings.HasPrefix(arg, "--command="):
			command = strings.TrimPrefix(arg, "--command=")
		case strings.HasPrefix(arg, "--args="):
			argsStr := strings.TrimPrefix(arg, "--args=")
			args = strings.Split(argsStr, ",")
		case strings.HasPrefix(arg, "--name="):
			name = strings.TrimPrefix(arg, "--name=")
		}
	}

	if serverType == "" || name == "" {
		fmt.Println("❌ Error: --type and --name are required")
		fmt.Println("Usage: agent-cli mcp add --type <http|stdio> --name <name> [options]")
		return
	}

	if serverType != "http" && serverType != "stdio" {
		fmt.Println("❌ Error: --type must be 'http' or 'stdio'")
		return
	}

	if serverType == "http" && url == "" {
		fmt.Println("❌ Error: --url is required for HTTP servers")
		return
	}

	if serverType == "stdio" && command == "" {
		fmt.Println("❌ Error: --command is required for stdio servers")
		return
	}

	// Load current configuration
	config := loadConfig()

	// Check if server with this name already exists
	for _, server := range config.MCPServers {
		if server.Name == name {
			fmt.Printf("❌ Error: MCP server with name '%s' already exists\n", name)
			return
		}
	}

	// Create new server config
	newServer := MCPServerConfig{
		Name: name,
		Type: serverType,
		Env:  make(map[string]string), // Initialize empty env map
	}

	if serverType == "http" {
		newServer.URL = url
	} else {
		newServer.Command = command
		newServer.Args = args
	}

	// Add server to configuration
	config.MCPServers = append(config.MCPServers, newServer)

	// Save configuration
	saveConfig(config)

	fmt.Printf("✅ Added MCP server '%s' (%s)\n", name, serverType)
	if serverType == "http" {
		fmt.Printf("   URL: %s\n", url)
	} else {
		fmt.Printf("   Command: %s\n", command)
		if len(args) > 0 {
			fmt.Printf("   Args: %v\n", args)
		}
	}
}

func listMCPServers() {
	config := loadConfig()

	if len(config.MCPServers) == 0 {
		fmt.Println("No MCP servers configured.")
		fmt.Println("Use 'agent-cli mcp add' to add a server.")
		return
	}

	fmt.Println("Configured MCP Servers:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	for i, server := range config.MCPServers {
		fmt.Printf("%d. %s (%s)\n", i+1, server.Name, server.Type)

		if server.Type == "http" {
			fmt.Printf("   URL: %s\n", server.URL)
		} else {
			fmt.Printf("   Command: %s\n", server.Command)
			if len(server.Args) > 0 {
				fmt.Printf("   Args: %v\n", server.Args)
			}
		}

		if len(server.Env) > 0 {
			fmt.Printf("   Environment Variables:\n")
			for key, value := range server.Env {
				fmt.Printf("     %s=%s\n", key, value)
			}
		}
		fmt.Println()
	}
}

func removeMCPServer() {
	var name string

	// Parse command line arguments
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
		}
	}

	if name == "" {
		fmt.Println("❌ Error: --name is required")
		fmt.Println("Usage: agent-cli mcp remove --name <name>")
		return
	}

	// Load current configuration
	config := loadConfig()

	// Find and remove server
	found := false
	newServers := []MCPServerConfig{}

	for _, server := range config.MCPServers {
		if server.Name != name {
			newServers = append(newServers, server)
		} else {
			found = true
		}
	}

	if !found {
		fmt.Printf("❌ Error: MCP server with name '%s' not found\n", name)
		return
	}

	config.MCPServers = newServers
	saveConfig(config)

	fmt.Printf("✅ Removed MCP server '%s'\n", name)
}

func testMCPServer() {
	var name string

	// Parse command line arguments
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--name=") {
			name = strings.TrimPrefix(arg, "--name=")
		}
	}

	if name == "" {
		fmt.Println("❌ Error: --name is required")
		fmt.Println("Usage: agent-cli mcp test --name <name>")
		return
	}

	// Load current configuration
	config := loadConfig()

	// Find server
	var targetServer *MCPServerConfig
	for _, server := range config.MCPServers {
		if server.Name == name {
			targetServer = &server
			break
		}
	}

	if targetServer == nil {
		fmt.Printf("❌ Error: MCP server with name '%s' not found\n", name)
		return
	}

	fmt.Printf("🧪 Testing MCP server '%s'...\n", name)

	// Create MCP server instance
	var mcpServer interfaces.MCPServer
	var err error

	ctx := context.Background()

	if targetServer.Type == "http" {
		mcpServer, err = mcp.NewHTTPServer(ctx, mcp.HTTPServerConfig{
			BaseURL: targetServer.URL,
		})
	} else {
		mcpServer, err = mcp.NewStdioServer(ctx, mcp.StdioServerConfig{
			Command: targetServer.Command,
			Args:    targetServer.Args,
		})
	}

	if err != nil {
		fmt.Printf("❌ Failed to create MCP server: %v\n", err)
		return
	}

	// Test by listing tools
	tools, err := mcpServer.ListTools(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to list tools: %v\n", err)
		return
	}

	fmt.Printf("✅ Connection successful!\n")
	fmt.Printf("📋 Available tools (%d):\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("   - %s: %s\n", tool.Name, tool.Description)
	}
}

func importMCPServers() {
	var filePath string

	// Parse command line arguments
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--file=") {
			filePath = strings.TrimPrefix(arg, "--file=")
		}
	}

	if filePath == "" {
		fmt.Println("❌ Error: --file is required")
		fmt.Println("Usage: agent-cli mcp import --file <path-to-json-config>")
		return
	}

	fmt.Printf("📥 Importing MCP servers from: %s\n", filePath)

	// Read JSON file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("❌ Failed to read file: %v\n", err)
		return
	}

	// Parse JSON
	var mcpConfig MCPServersConfig
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		fmt.Printf("❌ Failed to parse JSON: %v\n", err)
		return
	}

	// Load current configuration
	config := loadConfig()

	// Convert and add servers
	imported := 0
	skipped := 0

	for serverName, serverDef := range mcpConfig.MCPServers {
		// Check if server already exists
		exists := false
		for _, existing := range config.MCPServers {
			if existing.Name == serverName {
				exists = true
				break
			}
		}

		if exists {
			fmt.Printf("⚠️  Skipping '%s' - already exists\n", serverName)
			skipped++
			continue
		}

		// Determine server type
		serverType := "stdio" // default
		if serverDef.URL != "" {
			serverType = "http"
		}

		// Create new server config
		newServer := MCPServerConfig{
			Name:    serverName,
			Type:    serverType,
			Command: serverDef.Command,
			Args:    serverDef.Args,
			URL:     serverDef.URL,
			Env:     serverDef.Env,
		}

		// Initialize Env map if nil
		if newServer.Env == nil {
			newServer.Env = make(map[string]string)
		}

		config.MCPServers = append(config.MCPServers, newServer)
		imported++

		fmt.Printf("✅ Imported '%s' (%s)\n", serverName, serverType)
	}

	// Save configuration
	saveConfig(config)

	fmt.Printf("\n📊 Import Summary:\n")
	fmt.Printf("   ✅ Imported: %d servers\n", imported)
	if skipped > 0 {
		fmt.Printf("   ⚠️  Skipped: %d servers (already exist)\n", skipped)
	}
}

func exportMCPServers() {
	var filePath string

	// Parse command line arguments
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--file=") {
			filePath = strings.TrimPrefix(arg, "--file=")
		}
	}

	if filePath == "" {
		fmt.Println("❌ Error: --file is required")
		fmt.Println("Usage: agent-cli mcp export --file <path-to-json-config>")
		return
	}

	// Load current configuration
	config := loadConfig()

	if len(config.MCPServers) == 0 {
		fmt.Println("❌ No MCP servers configured to export")
		return
	}

	fmt.Printf("📤 Exporting MCP servers to: %s\n", filePath)

	// Convert to export format
	exportConfig := MCPServersConfig{
		MCPServers: make(map[string]MCPServerDefinition),
	}

	for _, server := range config.MCPServers {
		serverDef := MCPServerDefinition{
			Command: server.Command,
			Args:    server.Args,
			Env:     server.Env,
		}

		if server.Type == "http" {
			serverDef.URL = server.URL
		}

		// Initialize Env if nil
		if serverDef.Env == nil {
			serverDef.Env = make(map[string]string)
		}

		exportConfig.MCPServers[server.Name] = serverDef
	}

	// Marshal to JSON with pretty formatting
	data, err := json.MarshalIndent(exportConfig, "", "  ")
	if err != nil {
		fmt.Printf("❌ Failed to marshal JSON: %v\n", err)
		return
	}

	// Write to file with secure permissions
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		fmt.Printf("❌ Failed to write file: %v\n", err)
		return
	}

	fmt.Printf("✅ Exported %d MCP servers to %s\n", len(config.MCPServers), filePath)
}



func generateConfigs() {
	var systemPrompt, outputDir string

	// Parse command line arguments
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case strings.HasPrefix(arg, "--prompt="):
			systemPrompt = strings.TrimPrefix(arg, "--prompt=")
		case strings.HasPrefix(arg, "--output="):
			outputDir = strings.TrimPrefix(arg, "--output=")
		}
	}

	if systemPrompt == "" {
		fmt.Println("Usage: agent-cli generate --prompt=\"<system_prompt>\" [--output=<directory>]")
		fmt.Println("Example: agent-cli generate --prompt=\"You are a travel advisor\" --output=./configs")
		return
	}

	if outputDir == "" {
		outputDir = "."
	}

	fmt.Printf("🎨 Generating configurations from system prompt...\n")
	fmt.Printf("📝 Prompt: %s\n", systemPrompt)
	fmt.Printf("📁 Output directory: %s\n", outputDir)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	config := loadConfig()
	llm := createLLM(config)

	// Generate configurations
	ctx := context.Background()
	agentConfig, taskConfigs, err := agent.GenerateConfigFromSystemPrompt(ctx, llm, systemPrompt)
	if err != nil {
		log.Fatalf("❌ Failed to generate configurations: %v", err)
	}

	// Create output directory with secure permissions
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		log.Fatalf("❌ Failed to create output directory: %v", err)
	}

	// Save agent config
	agentConfigMap := map[string]agent.AgentConfig{
		"generated_agent": agentConfig,
	}

	agentFile := filepath.Join(outputDir, "agents.yaml")
	agentYaml, err := os.Create(agentFile)
	if err != nil {
		log.Fatalf("❌ Failed to create agent config file: %v", err)
	}
	defer func() {
		if closeErr := agentYaml.Close(); closeErr != nil {
			log.Printf("⚠️ Failed to close agent config file: %v", closeErr)
		}
	}()

	if err := agent.SaveAgentConfigsToFile(agentConfigMap, agentYaml); err != nil {
		log.Fatalf("❌ Failed to save agent configurations: %v", err)
	}

	// Save task configs
	taskFile := filepath.Join(outputDir, "tasks.yaml")
	taskYaml, err := os.Create(taskFile)
	if err != nil {
		log.Fatalf("❌ Failed to create task config file: %v", err)
	}
	defer func() {
		if closeErr := taskYaml.Close(); closeErr != nil {
			log.Printf("⚠️ Failed to close task config file: %v", closeErr)
		}
	}()

	// Convert slice to map for saving
	taskConfigMap := make(agent.TaskConfigs)
	for i, taskConfig := range taskConfigs {
		taskName := fmt.Sprintf("task_%d", i+1)
		taskConfigMap[taskName] = taskConfig
	}

	if err := agent.SaveTaskConfigsToFile(taskConfigMap, taskYaml); err != nil {
		log.Fatalf("❌ Failed to save task configurations: %v", err)
	}

	fmt.Printf("✅ Generated configurations:\n")
	fmt.Printf("   📄 Agent config: %s\n", agentFile)
	fmt.Printf("   📄 Task config: %s\n", taskFile)
	fmt.Printf("   🎯 Generated %d tasks\n", len(taskConfigs))

	fmt.Println("\n📝 Generated Agent Profile:")
	fmt.Printf("   Role: %s\n", agentConfig.Role)
	fmt.Printf("   Goal: %s\n", agentConfig.Goal)
	fmt.Printf("   Backstory: %s\n", agentConfig.Backstory)

	fmt.Println("\n🎯 Generated Tasks:")
	for taskName, taskConfig := range taskConfigMap {
		fmt.Printf("   - %s: %s\n", taskName, taskConfig.Description)
	}
}

func listResources() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: agent-cli list <resource>")
		fmt.Println("Available resources: providers, models, tools, config")
		return
	}

	resource := os.Args[2]
	switch resource {
	case "providers":
		listProviders()
	case "models":
		listModels()
	case "tools":
		listTools()
	case "config":
		showConfig()
	default:
		fmt.Printf("Unknown resource: %s\n", resource)
		fmt.Println("Available resources: providers, models, tools, config")
	}
}

func listProviders() {
	fmt.Println("Available LLM Providers:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  openai     - OpenAI GPT models (requires OPENAI_API_KEY)")
	fmt.Println("  anthropic  - Anthropic Claude models (requires ANTHROPIC_API_KEY)")
	fmt.Println("  vertex     - Google Vertex AI Gemini models (requires GOOGLE_APPLICATION_CREDENTIALS)")
	fmt.Println("  ollama     - Local Ollama server (requires Ollama running on localhost:11434)")
	fmt.Println("  vllm       - vLLM inference server (requires vLLM server running on localhost:8000)")
}

func listModels() {
	fmt.Println("Popular Models by Provider:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("OpenAI:")
	fmt.Println("  - gpt-4o-mini (fast, cost-effective)")
	fmt.Println("  - gpt-4o (most capable)")
	fmt.Println("  - gpt-3.5-turbo (legacy)")
	fmt.Println()
	fmt.Println("Anthropic:")
	fmt.Println("  - claude-3-5-sonnet-20241022 (most capable)")
	fmt.Println("  - claude-3-haiku-20240307 (fast)")
	fmt.Println()
	fmt.Println("Vertex AI:")
	fmt.Println("  - gemini-1.5-pro (most capable)")
	fmt.Println("  - gemini-1.5-flash (fast)")
	fmt.Println("  - gemini-2.0-flash-exp (experimental)")
	fmt.Println()
	fmt.Println("Ollama (local):")
	fmt.Println("  - llama3.2:latest")
	fmt.Println("  - mistral:latest")
	fmt.Println("  - codellama:latest")
}

func listTools() {
	fmt.Println("Available Tools:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Built-in Tools:")
	fmt.Println("  - websearch    - Google Custom Search (requires GOOGLE_API_KEY, GOOGLE_SEARCH_ENGINE_ID)")
	fmt.Println("  - github       - GitHub repository access (requires GITHUB_TOKEN)")
	fmt.Println("  - calculator   - Basic mathematical calculations")
	fmt.Println()
	fmt.Println("MCP Servers:")
	fmt.Println("  - HTTP servers - Connect to MCP servers via HTTP")
	fmt.Println("  - Stdio servers - Connect to MCP servers via stdio")
	fmt.Println()
	fmt.Println("Agent Tools:")
	fmt.Println("  - Sub-agents can be used as tools for delegation")
}

// Helper functions

// loadEnvFile loads environment variables from .env file if it exists
func loadEnvFile() {
	envFile := ".env"
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		return // .env file doesn't exist, that's okay
	}

	file, err := os.Open(envFile)
	if err != nil {
		return // Can't open .env file, continue without it
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("⚠️ Failed to close .env file: %v", closeErr)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
			(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
			value = value[1 : len(value)-1]
		}

		// Only set if not already set in environment
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				log.Printf("⚠️ Failed to set environment variable %s: %v", key, err)
			}
		}
	}
}

func loadConfig() *CLIConfig {
	configFile := filepath.Join(getConfigDir(), "config.json")

	// Return default config if file doesn't exist
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		defaultProvider := "openai"
		if envProvider := os.Getenv("LLM_PROVIDER"); envProvider != "" {
			defaultProvider = envProvider
		}

		return &CLIConfig{
			Provider:       defaultProvider,
			Model:          getDefaultModel(defaultProvider),
			SystemPrompt:   "You are a helpful AI assistant.",
			Temperature:    0.7,
			MaxIterations:  2,
			OrgID:          "default-org",
			ConversationID: "cli-session",
			EnableTracing:  false,
			EnableMemory:   true,
			EnableTools:    true,
			Variables:      make(map[string]string),
		}
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config CLIConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	// Override with environment variables if set
	if envProvider := os.Getenv("LLM_PROVIDER"); envProvider != "" {
		config.Provider = envProvider
		// Update model to default for the new provider if not explicitly set
		if config.Model == "" || config.Model == getDefaultModel(config.Provider) {
			config.Model = getDefaultModel(envProvider)
		}
	}

	return &config
}

func saveConfig(config *CLIConfig) {
	configFile := filepath.Join(getConfigDir(), "config.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}
}

func getConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get home directory: %v", err)
	}
	return filepath.Join(homeDir, ".agent-cli")
}

func getDefaultModel(provider string) string {
	switch provider {
	case "openai":
		return "gpt-4o-mini"
	case "anthropic":
		return "claude-3-5-sonnet-20241022"
	case "vertex":
		return "gemini-1.5-pro"
	case "ollama":
		return "llama3.2:latest"
	case "vllm":
		return "meta-llama/Llama-2-7b-chat-hf"
	default:
		return "gpt-4o-mini"
	}
}

func createAgent(config *CLIConfig) *agent.Agent {
	llm := createLLM(config)

	options := []agent.Option{
		agent.WithLLM(llm),
		agent.WithSystemPrompt(config.SystemPrompt),
		agent.WithMaxIterations(config.MaxIterations),
		agent.WithName("CLI-Agent"),
	}

	// Add memory if enabled
	if config.EnableMemory {
		options = append(options, agent.WithMemory(memory.NewConversationBuffer()))
	}

	// Add tools if enabled
	if config.EnableTools {
		tools := createTools()
		if len(tools) > 0 {
			options = append(options, agent.WithTools(tools...))
		}
	}

	// Add lazy MCP configurations if configured
	if len(config.MCPServers) > 0 {
		lazyMCPConfigs := createLazyMCPConfigs(config.MCPServers, nil) // No tool filtering for regular agent creation
		if len(lazyMCPConfigs) > 0 {
			options = append(options, agent.WithLazyMCPConfigs(lazyMCPConfigs))
		}
	}

	// Add tracing if enabled
	if config.EnableTracing {
		tracer := createTracer()
		if tracer != nil {
			options = append(options, agent.WithTracer(tracer))
		}
	}

	agentInstance, err := agent.NewAgent(options...)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	return agentInstance
}

func createLLM(config *CLIConfig) interfaces.LLM {
	switch config.Provider {
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			log.Fatal("OPENAI_API_KEY environment variable is required for OpenAI provider")
		}
		return openai.NewClient(apiKey,
			openai.WithModel(config.Model))

	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			log.Fatal("ANTHROPIC_API_KEY environment variable is required for Anthropic provider")
		}
		return anthropic.NewClient(apiKey,
			anthropic.WithModel(config.Model))

	case "vertex":
		projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			log.Fatal("GOOGLE_CLOUD_PROJECT environment variable is required for Vertex AI provider")
		}
		client, err := vertex.NewClient(context.Background(), projectID,
			vertex.WithModel(config.Model))
		if err != nil {
			log.Fatalf("Failed to create Vertex AI client: %v", err)
		}
		return client

	case "ollama":
		baseURL := os.Getenv("OLLAMA_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return ollama.NewClient(
			ollama.WithBaseURL(baseURL),
			ollama.WithModel(config.Model))

	case "vllm":
		baseURL := os.Getenv("VLLM_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8000"
		}
		return vllm.NewClient(
			vllm.WithBaseURL(baseURL),
			vllm.WithModel(config.Model))

	default:
		log.Fatalf("Unknown LLM provider: %s", config.Provider)
		return nil
	}
}

func createTools() []interfaces.Tool {
	var toolList []interfaces.Tool
	cfg := config.Get()

	// Web search tool
	if cfg.Tools.WebSearch.GoogleAPIKey != "" && cfg.Tools.WebSearch.GoogleSearchEngineID != "" {
		searchTool := websearch.New(
			cfg.Tools.WebSearch.GoogleAPIKey,
			cfg.Tools.WebSearch.GoogleSearchEngineID,
		)
		toolList = append(toolList, searchTool)
	}

	// GitHub tool
	if cfg.Tools.GitHub.Token != "" {
		githubTool, err := github.NewGitHubContentExtractorTool(cfg.Tools.GitHub.Token)
		if err == nil {
			toolList = append(toolList, githubTool)
		}
	}

	return toolList
}

func createLazyMCPConfigs(configs []MCPServerConfig, allowedTools []string) []agent.LazyMCPConfig {
	var lazyConfigs []agent.LazyMCPConfig

	for _, config := range configs {
		// Convert environment variables from map[string]string to []string format
		var envVars []string
		if len(config.Env) > 0 {
			for key, value := range config.Env {
				envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
			}
			log.Printf("Configured %d environment variables for lazy MCP server '%s'", len(envVars), config.Name)
		}

		// Create lazy MCP config that will discover tools dynamically
		// We'll create a special dynamic discovery approach
		lazyConfig := agent.LazyMCPConfig{
			Name:    config.Name,
			Type:    config.Type,
			Command: config.Command,
			Args:    config.Args,
			Env:     envVars,
			URL:     config.URL,
			Tools:   createDynamicMCPTools(config.Name, allowedTools), // Create dynamic discovery tools
		}

		lazyConfigs = append(lazyConfigs, lazyConfig)
	}

	return lazyConfigs
}

// createDynamicMCPTools creates placeholder tools that will discover actual schemas dynamically
func createDynamicMCPTools(serverName string, allowedTools []string) []agent.LazyMCPToolConfig {
	// If allowedTools is empty, don't create any tools
	if len(allowedTools) == 0 {
		return []agent.LazyMCPToolConfig{}
	}

	// Create placeholder tools for each allowed tool - schemas will be discovered dynamically
	var tools []agent.LazyMCPToolConfig
	for _, toolName := range allowedTools {
		tools = append(tools, agent.LazyMCPToolConfig{
			Name:        toolName,
			Description: fmt.Sprintf("MCP tool: %s", toolName),
			Schema:      nil, // Will be discovered dynamically from the MCP server
		})
	}

	return tools
}

func createTracer() interfaces.Tracer {
	cfg := config.Get()

	if cfg.Tracing.Langfuse.Enabled {
		langfuseConfig := tracing.LangfuseConfig{
			SecretKey:   cfg.Tracing.Langfuse.SecretKey,
			PublicKey:   cfg.Tracing.Langfuse.PublicKey,
			Host:        cfg.Tracing.Langfuse.Host,
			Environment: cfg.Tracing.Langfuse.Environment,
		}
		tracer, err := tracing.NewLangfuseTracer(langfuseConfig)
		if err != nil {
			log.Printf("Failed to create Langfuse tracer: %v", err)
			return nil
		}
		// Use the AsInterfaceTracer method to get interfaces.Tracer compatible adapter
		return tracer.AsInterfaceTracer()
	}

	return nil
}

func createContext(config *CLIConfig) context.Context {
	ctx := context.Background()

	// Add organization ID
	ctx = multitenancy.WithOrgID(ctx, config.OrgID)

	// Add conversation ID for memory
	ctx = context.WithValue(ctx, memory.ConversationIDKey, config.ConversationID)

	return ctx
}
