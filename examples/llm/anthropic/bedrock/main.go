package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/aws/aws-sdk-go-v2/config"
)

func main() {
	ctx := context.Background()

	// Example 1: Using AWS Default Credential Chain
	// This will automatically use credentials from:
	// - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
	// - ~/.aws/credentials file
	// - IAM role (if running on EC2/ECS/Lambda)
	fmt.Println("=== Example 1: AWS Default Credential Chain ===")
	awsConfig1, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v\n", err)
	}

	client1 := anthropic.NewClient("",
		anthropic.WithModel("us.anthropic.claude-opus-4-5-20251101-v1:0"),
		anthropic.WithBedrockAWSConfig(awsConfig1),
	)

	response1, err := client1.Generate(ctx, "Write a haiku about cloud computing")
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Response: %s\n\n", response1)
	}

	// Example 2: Using AWS Config with Custom Settings
	fmt.Println("=== Example 2: AWS Config with Custom Settings ===")
	awsConfig2, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithRetryMaxAttempts(5),
		// You can add more custom options:
		// config.WithHTTPClient(customHTTPClient),
		// config.WithCredentialsProvider(...),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v\n", err)
	}

	client2 := anthropic.NewClient("",
		anthropic.WithModel("us.anthropic.claude-opus-4-5-20251101-v1:0"),
		anthropic.WithBedrockAWSConfig(awsConfig2),
	)

	response2, err := client2.Generate(ctx, "Explain quantum computing in one sentence.")
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Response: %s\n\n", response2)
	}

	// Example 3: Using Automatic Model Conversion
	fmt.Println("=== Example 3: Automatic Model Conversion ===")
	// The SDK will automatically convert standard Anthropic model names to Bedrock format
	awsConfig3, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v\n", err)
	}

	client3 := anthropic.NewClient("",
		anthropic.WithModel("us.anthropic.claude-opus-4-5-20251101-v1:0"), // Automatically converts to Bedrock format
		anthropic.WithBedrockAWSConfig(awsConfig3),
	)

	response3, err := client3.Generate(ctx, "List 3 benefits of cloud computing.")
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Response: %s\n\n", response3)
	}

	// Example 4: Streaming Responses
	fmt.Println("=== Example 4: Streaming Responses ===")
	fmt.Println("Question: Explain the benefits of serverless computing in 3 points")
	fmt.Println("\nStreaming response:")
	fmt.Println("---")

	awsConfig4, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v\n", err)
	}

	client4 := anthropic.NewClient("",
		anthropic.WithModel("us.anthropic.claude-opus-4-5-20251101-v1:0"),
		anthropic.WithBedrockAWSConfig(awsConfig4),
	)

	// Start streaming - returns a channel of events
	eventsChan, err := client4.GenerateStream(ctx, "Explain the benefits of serverless computing in 3 points")
	if err != nil {
		log.Fatalf("Error starting stream: %v\n", err)
	}

	// Process streaming events as they arrive
	var fullResponse string
	for event := range eventsChan {
		switch event.Type {
		case interfaces.StreamEventMessageStart:
			// Message started
			continue
		case interfaces.StreamEventContentDelta:
			// Print each chunk as it arrives
			fmt.Print(event.Content)
			fullResponse += event.Content
		case interfaces.StreamEventContentComplete:
			// Content completed
			continue
		case interfaces.StreamEventMessageStop:
			// Message completed
			fmt.Println("\n---")
			fmt.Printf("Full response length: %d characters\n\n", len(fullResponse))
		case interfaces.StreamEventError:
			log.Printf("Stream error: %v\n", event.Error)
		}
	}

	// Example 5: Streaming with Extended Thinking (Opus 4.5)
	fmt.Println("=== Example 5: Streaming with Extended Thinking ===")
	fmt.Println("Question: Solve this logic puzzle: If all roses are flowers and some flowers fade quickly, what can we conclude?")
	fmt.Println("\nStreaming response with thinking:")
	fmt.Println("---")

	awsConfig5, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v\n", err)
	}

	client5 := anthropic.NewClient("",
		anthropic.WithModel("us.anthropic.claude-opus-4-5-20251101-v1:0"),
		anthropic.WithBedrockAWSConfig(awsConfig5),
	)

	// Start streaming - returns a channel of events
	eventsChan5, err := client5.GenerateStream(ctx, "Solve this logic puzzle: If all roses are flowers and some flowers fade quickly, what can we conclude?")
	if err != nil {
		log.Fatalf("Error starting stream: %v\n", err)
	}

	// Process streaming events as they arrive, including thinking events
	var fullResponse5 string
	var thinkingContent string
	for event := range eventsChan5 {
		switch event.Type {
		case interfaces.StreamEventMessageStart:
			// Message started
			continue
		case interfaces.StreamEventThinking:
			// Extended thinking content - shows the model's reasoning process
			fmt.Print(event.Content)
			thinkingContent += event.Content
		case interfaces.StreamEventContentDelta:
			// Print each chunk as it arrives
			fmt.Print(event.Content)
			fullResponse5 += event.Content
		case interfaces.StreamEventContentComplete:
			// Content completed
			continue
		case interfaces.StreamEventMessageStop:
			// Message completed
			fmt.Println("\n---")
			if len(thinkingContent) > 0 {
				fmt.Printf("Thinking tokens: %d characters\n", len(thinkingContent))
			}
			fmt.Printf("Response length: %d characters\n\n", len(fullResponse5))
		case interfaces.StreamEventError:
			log.Printf("Stream error: %v\n", event.Error)
		}
	}

	fmt.Println("=== All Examples Completed ===")
}
