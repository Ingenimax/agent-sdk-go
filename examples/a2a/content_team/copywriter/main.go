// Package main runs a Copywriter A2A agent that drafts social media posts.
//
// Start with:
//
//	OPENAI_API_KEY=sk-... go run ./examples/a2a/content_team/copywriter
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

const systemPrompt = `You are an expert social media copywriter.
Given a topic or product announcement, write engaging posts for three platforms:

1. Twitter/X -- max 280 characters, punchy and attention-grabbing
2. LinkedIn -- professional tone, 1-2 paragraphs, value-focused
3. Instagram -- casual, visual-oriented, include emoji placeholders where images would go

Write all three in a single response, clearly labeled by platform.`

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
		agent.WithName("Copywriter"),
		agent.WithDescription("Creative social media copywriter that drafts platform-specific posts"),
		agent.WithLLM(llm),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithRequirePlanApproval(false),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	card := a2apkg.NewCardBuilder(
		"Copywriter",
		"Creative social media copywriter that drafts platform-specific posts",
		"http://localhost:9110/",
		a2apkg.WithVersion("1.0.0"),
		a2apkg.WithStreaming(true),
	).AddSkill(a2a.AgentSkill{
		ID:          "draft-posts",
		Name:        "Draft Social Media Posts",
		Description: "Writes engaging posts for Twitter/X, LinkedIn, and Instagram given a topic",
		Tags:        []string{"copywriting", "social-media", "content"},
		Examples:    []string{"Write posts about the launch of our new AI code review tool"},
	}).Build()

	srv := a2apkg.NewServer(ag, card, a2apkg.WithAddress(":9110"))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Printf("Starting Copywriter agent on :9110")
	log.Printf("Agent card: http://localhost:9110/.well-known/agent-card.json")

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
