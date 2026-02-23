// Package main runs a Reviewer A2A agent that edits and improves social media posts.
//
// Start with:
//
//	OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/reviewer
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

const systemPrompt = `You are a senior content editor specializing in social media.
Review the provided social media posts for:

- Grammar and spelling
- Brand safety (no controversial or offensive language)
- Engagement potential (hooks, calls-to-action)
- Platform appropriateness (character limits, tone conventions)
- Tone consistency across platforms

Provide specific improvement suggestions and a revised version of each post.
Keep the same platform labels (Twitter/X, LinkedIn, Instagram) in your output.`

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
		agent.WithName("Reviewer"),
		agent.WithDescription("Senior content editor that reviews and improves social media posts"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	card := a2apkg.NewCardBuilder(
		"Reviewer",
		"Senior content editor that reviews and improves social media posts",
		"http://localhost:9111/",
		a2apkg.WithVersion("1.0.0"),
		a2apkg.WithStreaming(true),
	).AddSkill(a2a.AgentSkill{
		ID:          "review-posts",
		Name:        "Review Social Media Posts",
		Description: "Reviews posts for grammar, brand safety, engagement, and platform fit",
		Tags:        []string{"editing", "review", "content-quality"},
		Examples:    []string{"Review these Twitter, LinkedIn, and Instagram posts for quality"},
	}).Build()

	srv := a2apkg.NewServer(ag, card, a2apkg.WithAddress(":9111"))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Printf("Starting Reviewer agent on :9111")
	log.Printf("Agent card: http://localhost:9111/.well-known/agent-card.json")

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
