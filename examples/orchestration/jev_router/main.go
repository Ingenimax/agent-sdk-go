package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/jev"
	"github.com/Ingenimax/agent-sdk-go/pkg/orchestration"
)

func main() {
	apiKey := os.Getenv("TYPESAFE_API_KEY")
	if apiKey == "" {
		log.Fatal("TYPESAFE_API_KEY is required")
	}

	client := jev.NewClient(apiKey)
	router, err := orchestration.NewJevRouter(
		client,
		orchestration.WithJevMinimumConfidence(0.8),
	)
	if err != nil {
		log.Fatal(err)
	}

	// A deadline bounds the whole call, including any retry backoff the API asks for.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	decision, err := router.RouteDetailed(
		ctx,
		"The deployment is returning 500s after today's release",
		map[string]interface{}{
			"agents": map[string]string{
				"support":  "General customer questions",
				"incident": "Production outages and regressions",
				"billing":  "Invoices, payments, and refunds",
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("agent=%s confidence=%.2f model=%s\n", decision.AgentID, decision.Confidence, decision.Model)
}
