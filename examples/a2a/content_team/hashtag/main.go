// Package main runs a Hashtag Guru A2A agent that optimizes posts with hashtags and tips.
//
// Start with:
//
//	OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/hashtag
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/a2aproject/a2a-go/a2a"

	a2apkg "github.com/Ingenimax/agent-sdk-go/pkg/a2a"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

const systemPrompt = `You are a social media strategist and hashtag expert.
Given social media posts, provide:

1. Relevant hashtags (5-8 per platform), ranked by reach potential
2. Suggested optimal posting times (in EST) for each platform
3. Platform-specific optimization tips (e.g., thread format for Twitter, carousel suggestion for Instagram)

Format your output clearly per platform (Twitter/X, LinkedIn, Instagram).
Include the final optimized version of each post with hashtags integrated.`

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}

	model := "gpt-4o-mini"
	if m := os.Getenv("OPENAI_MODEL"); m != "" {
		model = m
	}

	llm := openai.NewClient(apiKey, openai.WithModel(model))

	ag, err := agent.NewAgent(
		agent.WithName("HashtagGuru"),
		agent.WithDescription("Social media strategist that adds hashtags and platform optimization tips"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	card := a2apkg.NewCardBuilder(
		"HashtagGuru",
		"Social media strategist that adds hashtags and platform optimization tips",
		"http://localhost:9112/",
		a2apkg.WithVersion("1.0.0"),
		a2apkg.WithStreaming(true),
	).AddSkill(a2a.AgentSkill{
		ID:          "optimize-posts",
		Name:        "Optimize Posts with Hashtags",
		Description: "Generates hashtags, posting times, and platform-specific optimization tips",
		Tags:        []string{"hashtags", "optimization", "strategy"},
		Examples:    []string{"Add hashtags and optimization tips to these social media posts"},
	}).Build()

	srv := a2apkg.NewServer(ag, card, a2apkg.WithAddress(":9112"))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Printf("Starting HashtagGuru agent on :9112")
	log.Printf("Agent card: http://localhost:9112/.well-known/agent-card.json")

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
