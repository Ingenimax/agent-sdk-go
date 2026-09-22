package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/jev"
	"github.com/Ingenimax/agent-sdk-go/pkg/orchestration"
)

func main() {
	apiKey := os.Getenv("TYPESAFE_API_KEY")
	if apiKey == "" {
		log.Fatal("TYPESAFE_API_KEY is required")
	}

	client := jev.NewClient(apiKey)
	router := orchestration.NewJevRouter(
		client,
		orchestration.WithJevMinimumConfidence(0.8),
	)

	decision, err := router.RouteDetailed(
		context.Background(),
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
