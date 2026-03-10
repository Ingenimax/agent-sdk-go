// Package main runs the Content Director that orchestrates a team of specialist
// A2A agents (Copywriter, Reviewer, Hashtag Guru) to produce polished social
// media posts for a given topic.
//
// Prerequisites -- start the three specialist agents first:
//
//	go run ./examples/a2a/content_team/copywriter
//	go run ./examples/a2a/content_team/reviewer
//	go run ./examples/a2a/content_team/hashtag
//
// Then run:
//
//	go run ./examples/a2a/content_team/orchestrator "Launch of our new AI code review tool"
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	a2apkg "github.com/Ingenimax/agent-sdk-go/pkg/a2a"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
)

const defaultTopic = "Launch of our new AI-powered code review tool that catches bugs before they reach production"

const directorPrompt = `You are a content director managing a social media content creation team.
You have three specialist agents available as tools:

- copywriter: Drafts engaging posts for Twitter/X, LinkedIn, and Instagram
- reviewer: Reviews and improves posts for quality, brand safety, and engagement
- hashtag_optimizer: Adds hashtags, posting times, and platform optimization tips

Your workflow for every request:
1. Send the topic to the copywriter to draft initial posts
2. Send the copywriter's drafts to the reviewer for editorial feedback and revision
3. Send the reviewer's revised posts to the hashtag_optimizer for final touches

Always follow this exact order. After all three steps, present the final polished result
to the user with a brief summary of the process.`

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	model := "gpt-4o-mini"
	if m := os.Getenv("OPENAI_MODEL"); m != "" {
		model = m
	}

	ctx := context.Background()

	// Discover and connect to the three specialist agents
	copyClient, err := a2apkg.NewClient(ctx, "http://localhost:9110")
	if err != nil {
		log.Fatalf("Failed to connect to Copywriter: %v", err)
	}

	reviewClient, err := a2apkg.NewClient(ctx, "http://localhost:9111")
	if err != nil {
		log.Fatalf("Failed to connect to Reviewer: %v", err)
	}

	hashtagClient, err := a2apkg.NewClient(ctx, "http://localhost:9112")
	if err != nil {
		log.Fatalf("Failed to connect to HashtagGuru: %v", err)
	}

	// Print discovered agents
	for _, c := range []*a2apkg.Client{copyClient, reviewClient, hashtagClient} {
		card := c.Card()
		fmt.Printf("Discovered: %s -- %s\n", card.Name, card.Description)
	}
	fmt.Println()

	// Wrap each A2A agent as a tool for the orchestrator
	copyTool := a2apkg.NewRemoteAgentTool(copyClient, a2apkg.WithToolName("copywriter"))
	reviewTool := a2apkg.NewRemoteAgentTool(reviewClient, a2apkg.WithToolName("reviewer"))
	hashtagTool := a2apkg.NewRemoteAgentTool(hashtagClient, a2apkg.WithToolName("hashtag_optimizer"))

	llm := openai.NewClient(apiKey, openai.WithModel(model))

	director, err := agent.NewAgent(
		agent.WithName("ContentDirector"),
		agent.WithDescription("Social media content creation director"),
		agent.WithLLM(llm),
		agent.WithTools(copyTool, reviewTool, hashtagTool),
		agent.WithSystemPrompt(directorPrompt),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithOrgID("content-team"),
		agent.WithMaxIterations(5),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Fatalf("Failed to create director agent: %v", err)
	}

	topic := defaultTopic
	if len(os.Args) > 1 {
		topic = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("Topic: %s\n\n", topic)

	// Set conversation ID required by the memory subsystem
	// (org ID is injected by the agent via WithOrgID)
	ctx = memory.WithConversationID(ctx, "content-session")

	result, err := director.Run(ctx, topic)
	if err != nil {
		log.Fatalf("Director failed: %v", err)
	}

	fmt.Println(result)
}
