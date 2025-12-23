// Package main demonstrates using GraphRAG as persistent memory for an agent.
// The agent can learn facts from conversations, store them in the knowledge graph,
// and recall them later - creating a form of long-term structured memory.
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag/weaviate"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	ctx := context.Background()

	// Set up tenant context for memory isolation
	// Each user/session can have their own memory space
	userID := os.Getenv("USER_ID")
	if userID == "" {
		userID = "demo-user"
	}
	ctx = multitenancy.WithOrgID(ctx, userID)

	// Set conversation ID for session-based memory
	// This identifies the current conversation session
	sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
	ctx = memory.WithConversationID(ctx, sessionID)

	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    GraphRAG Memory Agent Demo                                 ║")
	fmt.Println("║                                                                              ║")
	fmt.Println("║  This agent uses a knowledge graph as persistent memory.                     ║")
	fmt.Println("║  Tell it things, and it will remember them!                                  ║")
	fmt.Println("║                                                                              ║")
	fmt.Println("║  Try:                                                                        ║")
	fmt.Println("║  • \"My name is Alex and I work at Acme Corp as a software engineer\"          ║")
	fmt.Println("║  • \"I'm working on a project called Phoenix using Go and PostgreSQL\"         ║")
	fmt.Println("║  • \"My colleague Sarah is the tech lead on Phoenix\"                          ║")
	fmt.Println("║  • \"What do you know about me?\"                                              ║")
	fmt.Println("║  • \"What projects am I working on?\"                                          ║")
	fmt.Println("║  • \"Who else works on Phoenix?\"                                              ║")
	fmt.Println("║                                                                              ║")
	fmt.Println("║  Type 'quit' or 'exit' to end the session.                                   ║")
	fmt.Println("║  Type 'memory' to see what's stored in the knowledge graph.                  ║")
	fmt.Println("║  Type 'clear' to reset the memory.                                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("\n[Memory Space: %s]\n\n", userID)

	// Check for API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	// Initialize LLM
	llm := anthropic.NewClient(
		apiKey,
		anthropic.WithModel("claude-sonnet-4-20250514"),
	)

	// Initialize embedder for semantic search (optional but recommended)
	var embedder embedding.Client
	openAIKey := os.Getenv("OPENAI_API_KEY")
	if openAIKey != "" {
		embedder = embedding.NewOpenAIEmbedder(openAIKey, "text-embedding-3-small")
		log.Println("Using OpenAI embedder for semantic search")
	} else {
		log.Println("OPENAI_API_KEY not set - using keyword search only")
	}

	// Initialize GraphRAG store as memory backend
	weaviateURL := os.Getenv("WEAVIATE_URL")
	if weaviateURL == "" {
		weaviateURL = "localhost:8080"
	}

	storeOpts := []weaviate.Option{
		weaviate.WithStoreTenant(userID),
	}
	if embedder != nil {
		storeOpts = append(storeOpts, weaviate.WithEmbedder(embedder))
	}

	store, err := weaviate.New(&weaviate.Config{
		Host:        weaviateURL,
		Scheme:      "http",
		ClassPrefix: "Memory", // Collections: MemoryEntity, MemoryRelationship
	}, storeOpts...)
	if err != nil {
		log.Fatalf("Failed to create GraphRAG store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Create conversation memory for session context
	// This maintains chat history within the session
	conversationMemory := memory.NewConversationBuffer()

	// Create the memory-enabled agent
	// MaxIterations must be high enough for multi-step memory operations
	// (e.g., adding person + organization + relationship = 3+ tool calls)
	ag, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithName("MemoryAgent"),
		agent.WithMemory(conversationMemory),  // Short-term: conversation history
		agent.WithGraphRAG(store),              // Long-term: structured knowledge graph
		agent.WithRequirePlanApproval(false),
		agent.WithMaxIterations(10), // Allow enough iterations for memory operations
		agent.WithSystemPrompt(memoryAgentPrompt),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Interactive conversation loop
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\nYou: ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle special commands
		switch strings.ToLower(input) {
		case "quit", "exit":
			fmt.Println("\nGoodbye! Your memories are saved in the knowledge graph.")
			return

		case "memory", "show memory", "what do you remember":
			showMemory(ctx, store)
			continue

		case "clear", "forget everything", "reset":
			clearMemory(ctx, store)
			fmt.Println("\n🧹 Memory cleared. Starting fresh!")
			continue
		}

		// Process with agent
		response, err := ag.Run(ctx, input)
		if err != nil {
			fmt.Printf("\n❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("\nAgent: %s\n", response)
	}
}

const memoryAgentPrompt = `You are a helpful assistant with persistent memory capabilities. You can remember facts about the user, their work, relationships, and interests by storing them in a knowledge graph.

## Your Memory Capabilities

You have access to these memory tools:
- **graphrag_search**: Search your memory for entities and relationships
- **graphrag_add_entity**: Store a new fact/entity in memory
- **graphrag_add_relationship**: Store a connection between entities
- **graphrag_get_context**: Explore connections around an entity
- **graphrag_extract**: Automatically extract entities and relationships from text

## How to Use Your Memory

### When the user tells you something new:
1. Identify key entities (people, organizations, projects, skills, locations, etc.)
2. Use graphrag_add_entity to store each entity with a descriptive name and type
3. Use graphrag_add_relationship to connect related entities
4. Acknowledge that you've remembered the information

### When the user asks what you know:
1. Use graphrag_search to find relevant entities
2. Use graphrag_get_context to explore relationships
3. Synthesize the information into a natural response

### Entity Types to Use:
- **Person**: People (the user, colleagues, friends, family)
- **Organization**: Companies, teams, departments
- **Project**: Work projects, personal projects
- **Skill**: Technologies, programming languages, tools
- **Location**: Cities, countries, offices
- **Topic**: Interests, hobbies, areas of expertise
- **Event**: Meetings, milestones, important dates

### Relationship Types to Use:
- **WORKS_AT**: Person → Organization
- **WORKS_ON**: Person → Project
- **KNOWS**: Person → Person
- **COLLEAGUE_OF**: Person → Person (work relationship)
- **MANAGES**: Person → Person/Project
- **USES**: Project → Skill, or Person → Skill
- **INTERESTED_IN**: Person → Topic
- **LOCATED_IN**: Person/Organization → Location
- **PART_OF**: Project → Organization

### Best Practices:
1. Always search memory first when answering questions
2. Store entities with clear, specific names (e.g., "Alex Chen" not just "user")
3. Include context in entity descriptions
4. Create bidirectional relationships when appropriate (A knows B, B knows A)
5. Update entities if you learn new information (add to description)
6. Use consistent entity IDs (lowercase, hyphenated: "person-alex-chen")

### Example Workflow:

User: "I'm Alex, I work at TechCorp as a senior engineer on the Atlas project"

Your actions:
1. graphrag_add_entity: {id: "person-alex", name: "Alex", type: "Person", description: "Senior engineer, the user of this system"}
2. graphrag_add_entity: {id: "org-techcorp", name: "TechCorp", type: "Organization", description: "Technology company where Alex works"}
3. graphrag_add_entity: {id: "project-atlas", name: "Atlas Project", type: "Project", description: "Project Alex works on at TechCorp"}
4. graphrag_add_relationship: {source_id: "person-alex", target_id: "org-techcorp", type: "WORKS_AT"}
5. graphrag_add_relationship: {source_id: "person-alex", target_id: "project-atlas", type: "WORKS_ON"}

Response: "Nice to meet you, Alex! I've noted that you're a senior engineer at TechCorp working on the Atlas project. Feel free to tell me more about your work or ask me anything!"

Remember: Your memory persists across conversations. Always check what you already know before adding duplicate information.`

// showMemory displays all entities and relationships in the knowledge graph
func showMemory(ctx context.Context, store interfaces.GraphRAGStore) {
	fmt.Println("\n📚 Current Memory Contents:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Search for all entities (broad search)
	results, err := store.Search(ctx, "*", 50)
	if err != nil {
		// Try a different approach - search for common entity types
		results, err = store.Search(ctx, "person organization project skill", 50)
		if err != nil {
			fmt.Printf("Could not retrieve memory: %v\n", err)
			return
		}
	}

	if len(results) == 0 {
		fmt.Println("Memory is empty. Tell me something to remember!")
		return
	}

	// Group by type
	byType := make(map[string][]interfaces.GraphSearchResult)
	for _, r := range results {
		byType[r.Entity.Type] = append(byType[r.Entity.Type], r)
	}

	for entityType, entities := range byType {
		fmt.Printf("\n🏷️  %s:\n", entityType)
		for _, e := range entities {
			fmt.Printf("   • %s", e.Entity.Name)
			if e.Entity.Description != "" {
				fmt.Printf(" - %s", e.Entity.Description)
			}
			fmt.Println()

			// Show relationships
			if len(e.Path) > 0 {
				for _, rel := range e.Path {
					fmt.Printf("     ↳ %s → %s\n", rel.Type, rel.TargetID)
				}
			}
		}
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// clearMemory removes all entities for the current user
func clearMemory(ctx context.Context, store interfaces.GraphRAGStore) {
	// Search and delete all entities
	results, err := store.Search(ctx, "person organization project skill topic location event", 100)
	if err != nil {
		return
	}

	for _, r := range results {
		_ = store.DeleteEntity(ctx, r.Entity.ID)
	}
}
