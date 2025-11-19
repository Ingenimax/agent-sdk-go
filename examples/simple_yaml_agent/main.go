package main

import (
	"context"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	// Create LLM client
	llm := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Load and create agent
	configs, _ := agent.LoadAgentConfigsFromFile("agents.yaml")
	agentInstance, _ := agent.NewAgentFromConfig("file_analyzer", configs, nil, agent.WithLLM(llm))

	// Run agent with file analysis request
	result, err := agentInstance.Run(context.Background(), "Analyze the main.go file and tell me what it does")
	if err != nil {
		log.Fatal(err)
	}

	// Print result
	println(result)
}