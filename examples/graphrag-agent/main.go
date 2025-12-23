// Example program demonstrating an agent using GraphRAG to answer questions
// about entities and relationships stored in a knowledge graph.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag/weaviate"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	ctx := context.Background()

	// Set organization ID for multi-tenancy consistency.
	// The Anthropic LLM client automatically adds "default" orgID to context during tool execution,
	// so we need to use the same tenant when storing entities to ensure search results match.
	ctx = multitenancy.WithOrgID(ctx, "default")

	// Check for API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	// Create GraphRAG store (requires Weaviate running on localhost:8080)
	store, err := weaviate.New(&weaviate.Config{
		Host:        "localhost:8080",
		Scheme:      "http",
		ClassPrefix: "AgentDemo",
	})
	if err != nil {
		log.Fatalf("Failed to create GraphRAG store: %v", err)
	}
	defer func() { _ = store.Close() }()

	fmt.Println("=== GraphRAG Agent Demo (Complex Knowledge Graph) ===")
	fmt.Println("\n1. Populating knowledge graph with sample data...")

	// Create a rich set of entities representing a tech ecosystem
	entities := createEntities()
	relationships := createRelationships()

	// Log the tenant being used
	if orgID, err := multitenancy.GetOrgID(ctx); err == nil {
		fmt.Printf("   Storing entities with tenant: %q\n", orgID)
	}

	if err := store.StoreEntities(ctx, entities); err != nil {
		log.Fatalf("Failed to store entities: %v", err)
	}
	fmt.Printf("   Stored %d entities\n", len(entities))

	if err := store.StoreRelationships(ctx, relationships); err != nil {
		log.Fatalf("Failed to store relationships: %v", err)
	}
	fmt.Printf("   Stored %d relationships\n", len(relationships))

	// Give Weaviate time to index
	fmt.Println("\n   Waiting for Weaviate to index (2 seconds)...")
	time.Sleep(2 * time.Second)

	// Verify data
	fmt.Println("\n   Verifying stored data...")
	count, _ := store.CountAllEntities(ctx)
	fmt.Printf("   Total entities in knowledge graph: %d\n", count)

	testResults, _ := store.Search(ctx, "engineer", 5)
	fmt.Printf("   Search for 'engineer' returned %d results\n", len(testResults))

	fmt.Println("\n2. Creating agent with GraphRAG capabilities...")

	// Create LLM client
	llm := anthropic.NewClient(
		apiKey,
		anthropic.WithModel("claude-sonnet-4-20250514"),
	)

	// Create agent with GraphRAG
	ag, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithGraphRAG(store),
		agent.WithName("KnowledgeGraphAgent"),
		agent.WithRequirePlanApproval(false),
		agent.WithSystemPrompt(`You are a helpful assistant with access to a knowledge graph containing information about a tech company ecosystem.

The knowledge graph contains:
- People (employees with various roles: engineers, managers, executives, designers)
- Organizations (companies, departments, teams)
- Projects (software projects with different statuses)
- Technologies (programming languages, frameworks, tools)
- Locations (cities and countries)

Relationships include:
- WORKS_AT: Person works at an organization
- MANAGES: Person manages another person or team
- REPORTS_TO: Person reports to another person
- WORKS_ON: Person works on a project
- LEADS: Person leads a project or team
- USES: Project uses a technology
- COLLABORATES_WITH: Person collaborates with another person
- PARTNERS_WITH: Organization partners with another organization
- PART_OF: Team/Department is part of an organization
- HEADQUARTERED_IN: Organization is headquartered in a city
- HAS_OFFICE_IN: Organization has an office in a city
- BASED_IN: Person is based in a city
- CITY_IN: City is located in a country

When answering questions:
1. Use graphrag_search to find relevant entities (try different search terms if needed)
2. Use graphrag_get_context to explore relationships around an entity
3. Combine information from multiple searches to give comprehensive answers
4. Be specific about roles, relationships, and project details`),
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	fmt.Println("   Agent created with GraphRAG tools")

	// Demonstrate the agent answering questions
	fmt.Println("\n3. Asking the agent questions about the knowledge graph...")

	questions := []string{
		"Who is the CTO and what teams do they manage?",
		"What projects is the Platform team working on?",
		"Who are the senior engineers and what technologies do they use?",
		"Tell me about the AI Research project - who leads it and what technologies does it use?",
		"Which people collaborate with Sarah Chen?",
		"What is the organizational structure of TechCorp?",
		"What organizations are located in San Francisco?",
		"Which cities in the USA have tech companies?",
		"Where is AI Labs Inc headquartered and who works there?",
	}

	for i, question := range questions {
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Question %d: %s\n", i+1, question)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		response, err := ag.Run(ctx, question)
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}

		fmt.Printf("\nAgent Response:\n%s\n\n", response)
	}

	// Clean up
	fmt.Println("\n4. Cleaning up...")
	if err := store.DeleteSchema(ctx); err != nil {
		log.Printf("Warning: Failed to delete schema: %v", err)
	}
	fmt.Println("   Done!")

	fmt.Println("\n=== Demo Complete ===")
}

// createEntities creates a rich set of entities for the knowledge graph
func createEntities() []interfaces.Entity {
	now := time.Now()

	return []interfaces.Entity{
		// === PEOPLE - Executives ===
		{
			ID:          "person-alice-wang",
			Name:        "Alice Wang",
			Type:        "Person",
			Description: "Chief Executive Officer (CEO) of TechCorp with 20 years of industry experience",
			Properties:  map[string]interface{}{"role": "CEO", "department": "Executive", "years_experience": 20, "email": "alice.wang@techcorp.com"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-bob-martinez",
			Name:        "Bob Martinez",
			Type:        "Person",
			Description: "Chief Technology Officer (CTO) leading all engineering teams and technical strategy",
			Properties:  map[string]interface{}{"role": "CTO", "department": "Engineering", "years_experience": 15, "email": "bob.martinez@techcorp.com"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-carol-johnson",
			Name:        "Carol Johnson",
			Type:        "Person",
			Description: "VP of Product overseeing product strategy and roadmap",
			Properties:  map[string]interface{}{"role": "VP of Product", "department": "Product", "years_experience": 12},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === PEOPLE - Engineering Managers ===
		{
			ID:          "person-david-kim",
			Name:        "David Kim",
			Type:        "Person",
			Description: "Engineering Manager leading the Platform team, expert in distributed systems",
			Properties:  map[string]interface{}{"role": "Engineering Manager", "team": "Platform", "skills": []string{"Go", "Kubernetes", "AWS"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-emily-chen",
			Name:        "Emily Chen",
			Type:        "Person",
			Description: "Engineering Manager leading the AI/ML team, PhD in Machine Learning",
			Properties:  map[string]interface{}{"role": "Engineering Manager", "team": "AI/ML", "skills": []string{"Python", "PyTorch", "TensorFlow"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-frank-wilson",
			Name:        "Frank Wilson",
			Type:        "Person",
			Description: "Engineering Manager leading the Frontend team",
			Properties:  map[string]interface{}{"role": "Engineering Manager", "team": "Frontend", "skills": []string{"TypeScript", "React", "Next.js"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === PEOPLE - Senior Engineers ===
		{
			ID:          "person-sarah-chen",
			Name:        "Sarah Chen",
			Type:        "Person",
			Description: "Senior Software Engineer specializing in backend services and APIs, Go expert",
			Properties:  map[string]interface{}{"role": "Senior Software Engineer", "team": "Platform", "skills": []string{"Go", "gRPC", "PostgreSQL"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-michael-brown",
			Name:        "Michael Brown",
			Type:        "Person",
			Description: "Senior ML Engineer working on recommendation systems and NLP",
			Properties:  map[string]interface{}{"role": "Senior ML Engineer", "team": "AI/ML", "skills": []string{"Python", "PyTorch", "LLMs"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-jennifer-lee",
			Name:        "Jennifer Lee",
			Type:        "Person",
			Description: "Senior Frontend Engineer, React and accessibility expert",
			Properties:  map[string]interface{}{"role": "Senior Frontend Engineer", "team": "Frontend", "skills": []string{"TypeScript", "React", "CSS"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-james-taylor",
			Name:        "James Taylor",
			Type:        "Person",
			Description: "Senior DevOps Engineer managing cloud infrastructure and CI/CD pipelines",
			Properties:  map[string]interface{}{"role": "Senior DevOps Engineer", "team": "Platform", "skills": []string{"Kubernetes", "Terraform", "AWS"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === PEOPLE - Engineers ===
		{
			ID:          "person-anna-garcia",
			Name:        "Anna Garcia",
			Type:        "Person",
			Description: "Software Engineer on the Platform team, working on microservices",
			Properties:  map[string]interface{}{"role": "Software Engineer", "team": "Platform", "skills": []string{"Go", "Docker", "Redis"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-ryan-patel",
			Name:        "Ryan Patel",
			Type:        "Person",
			Description: "ML Engineer working on computer vision and image processing",
			Properties:  map[string]interface{}{"role": "ML Engineer", "team": "AI/ML", "skills": []string{"Python", "OpenCV", "TensorFlow"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "person-lisa-wong",
			Name:        "Lisa Wong",
			Type:        "Person",
			Description: "Frontend Engineer building user interfaces and components",
			Properties:  map[string]interface{}{"role": "Frontend Engineer", "team": "Frontend", "skills": []string{"JavaScript", "React", "Tailwind"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === PEOPLE - Designers ===
		{
			ID:          "person-mark-anderson",
			Name:        "Mark Anderson",
			Type:        "Person",
			Description: "Senior UX Designer creating user experiences and design systems",
			Properties:  map[string]interface{}{"role": "Senior UX Designer", "team": "Design", "skills": []string{"Figma", "User Research", "Prototyping"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === ORGANIZATIONS ===
		{
			ID:          "org-techcorp",
			Name:        "TechCorp",
			Type:        "Organization",
			Description: "Leading technology company specializing in AI-powered enterprise solutions",
			Properties:  map[string]interface{}{"industry": "Technology", "size": "500 employees", "founded": "2015", "headquarters": "San Francisco"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "org-ai-labs",
			Name:        "AI Labs Inc",
			Type:        "Organization",
			Description: "Research laboratory and TechCorp partner specializing in cutting-edge AI research",
			Properties:  map[string]interface{}{"industry": "Research", "size": "50 employees", "focus": "AI Research"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "org-cloudservices",
			Name:        "CloudServices Co",
			Type:        "Organization",
			Description: "Cloud infrastructure provider partnering with TechCorp",
			Properties:  map[string]interface{}{"industry": "Cloud Computing", "size": "1000 employees"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === TEAMS ===
		{
			ID:          "team-platform",
			Name:        "Platform Team",
			Type:        "Team",
			Description: "Backend infrastructure and platform services team responsible for core systems",
			Properties:  map[string]interface{}{"focus": "Backend Infrastructure", "size": 8},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "team-aiml",
			Name:        "AI/ML Team",
			Type:        "Team",
			Description: "Artificial Intelligence and Machine Learning team building intelligent features",
			Properties:  map[string]interface{}{"focus": "Machine Learning", "size": 6},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "team-frontend",
			Name:        "Frontend Team",
			Type:        "Team",
			Description: "Frontend development team building user interfaces and web applications",
			Properties:  map[string]interface{}{"focus": "User Interface", "size": 5},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "team-design",
			Name:        "Design Team",
			Type:        "Team",
			Description: "UX/UI Design team creating user experiences and visual designs",
			Properties:  map[string]interface{}{"focus": "User Experience", "size": 3},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === PROJECTS ===
		{
			ID:          "project-atlas",
			Name:        "Project Atlas",
			Type:        "Project",
			Description: "Core platform modernization initiative - migrating to microservices architecture",
			Properties:  map[string]interface{}{"status": "Active", "priority": "High", "start_date": "2024-01", "target_date": "2025-06"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "project-ai-research",
			Name:        "AI Research Initiative",
			Type:        "Project",
			Description: "Research project exploring large language models for enterprise applications",
			Properties:  map[string]interface{}{"status": "Active", "priority": "High", "budget": "$2M"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "project-dashboard",
			Name:        "Analytics Dashboard",
			Type:        "Project",
			Description: "Next-generation analytics dashboard with real-time data visualization",
			Properties:  map[string]interface{}{"status": "Active", "priority": "Medium", "version": "2.0"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "project-mobile-app",
			Name:        "Mobile App",
			Type:        "Project",
			Description: "Cross-platform mobile application for TechCorp products",
			Properties:  map[string]interface{}{"status": "Planning", "priority": "Medium", "platforms": []string{"iOS", "Android"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "project-security-audit",
			Name:        "Security Audit 2024",
			Type:        "Project",
			Description: "Annual security audit and compliance review",
			Properties:  map[string]interface{}{"status": "Completed", "priority": "High", "compliance": []string{"SOC2", "GDPR"}},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === TECHNOLOGIES ===
		{
			ID:          "tech-go",
			Name:        "Go",
			Type:        "Technology",
			Description: "Go programming language used for backend services",
			Properties:  map[string]interface{}{"category": "Programming Language", "version": "1.22"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "tech-python",
			Name:        "Python",
			Type:        "Technology",
			Description: "Python programming language used for ML and data science",
			Properties:  map[string]interface{}{"category": "Programming Language", "version": "3.11"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "tech-typescript",
			Name:        "TypeScript",
			Type:        "Technology",
			Description: "TypeScript for type-safe frontend development",
			Properties:  map[string]interface{}{"category": "Programming Language", "version": "5.0"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "tech-kubernetes",
			Name:        "Kubernetes",
			Type:        "Technology",
			Description: "Container orchestration platform for deploying services",
			Properties:  map[string]interface{}{"category": "Infrastructure", "provider": "EKS"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "tech-pytorch",
			Name:        "PyTorch",
			Type:        "Technology",
			Description: "Deep learning framework for ML model development",
			Properties:  map[string]interface{}{"category": "ML Framework", "version": "2.0"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "tech-react",
			Name:        "React",
			Type:        "Technology",
			Description: "Frontend JavaScript library for building user interfaces",
			Properties:  map[string]interface{}{"category": "Frontend Framework", "version": "18"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "tech-postgresql",
			Name:        "PostgreSQL",
			Type:        "Technology",
			Description: "Primary relational database for data storage",
			Properties:  map[string]interface{}{"category": "Database", "version": "15"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === COUNTRIES ===
		{
			ID:          "country-usa",
			Name:        "United States",
			Type:        "Country",
			Description: "United States of America, major technology hub",
			Properties:  map[string]interface{}{"code": "US", "continent": "North America"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "country-uk",
			Name:        "United Kingdom",
			Type:        "Country",
			Description: "United Kingdom, European technology center",
			Properties:  map[string]interface{}{"code": "UK", "continent": "Europe"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "country-germany",
			Name:        "Germany",
			Type:        "Country",
			Description: "Germany, leading European technology and engineering hub",
			Properties:  map[string]interface{}{"code": "DE", "continent": "Europe"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "country-japan",
			Name:        "Japan",
			Type:        "Country",
			Description: "Japan, major Asian technology innovation center",
			Properties:  map[string]interface{}{"code": "JP", "continent": "Asia"},
			CreatedAt:   now,
			UpdatedAt:   now,
		},

		// === CITIES ===
		{
			ID:          "city-san-francisco",
			Name:        "San Francisco",
			Type:        "City",
			Description: "San Francisco, California - heart of Silicon Valley and tech startup ecosystem",
			Properties:  map[string]interface{}{"state": "California", "timezone": "PST", "tech_hub": true},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "city-new-york",
			Name:        "New York",
			Type:        "City",
			Description: "New York City - major financial and technology center on the East Coast",
			Properties:  map[string]interface{}{"state": "New York", "timezone": "EST", "tech_hub": true},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "city-boston",
			Name:        "Boston",
			Type:        "City",
			Description: "Boston, Massachusetts - home to MIT, Harvard and a thriving AI research scene",
			Properties:  map[string]interface{}{"state": "Massachusetts", "timezone": "EST", "tech_hub": true},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "city-london",
			Name:        "London",
			Type:        "City",
			Description: "London, UK - major European financial and technology center",
			Properties:  map[string]interface{}{"timezone": "GMT", "tech_hub": true},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "city-berlin",
			Name:        "Berlin",
			Type:        "City",
			Description: "Berlin, Germany - European startup capital and tech innovation hub",
			Properties:  map[string]interface{}{"timezone": "CET", "tech_hub": true},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "city-tokyo",
			Name:        "Tokyo",
			Type:        "City",
			Description: "Tokyo, Japan - Asia's largest technology and innovation center",
			Properties:  map[string]interface{}{"timezone": "JST", "tech_hub": true},
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
}

// createRelationships creates a rich set of relationships for the knowledge graph
func createRelationships() []interfaces.Relationship {
	now := time.Now()

	return []interfaces.Relationship{
		// === WORKS_AT Relationships ===
		{ID: "rel-alice-techcorp", SourceID: "person-alice-wang", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Alice Wang is the CEO of TechCorp", Strength: 1.0, CreatedAt: now},
		{ID: "rel-bob-techcorp", SourceID: "person-bob-martinez", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Bob Martinez is the CTO of TechCorp", Strength: 1.0, CreatedAt: now},
		{ID: "rel-carol-techcorp", SourceID: "person-carol-johnson", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Carol Johnson is VP of Product at TechCorp", Strength: 1.0, CreatedAt: now},
		{ID: "rel-david-techcorp", SourceID: "person-david-kim", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "David Kim works at TechCorp as Engineering Manager", Strength: 1.0, CreatedAt: now},
		{ID: "rel-emily-techcorp", SourceID: "person-emily-chen", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Emily Chen works at TechCorp as Engineering Manager", Strength: 1.0, CreatedAt: now},
		{ID: "rel-frank-techcorp", SourceID: "person-frank-wilson", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Frank Wilson works at TechCorp as Engineering Manager", Strength: 1.0, CreatedAt: now},
		{ID: "rel-sarah-techcorp", SourceID: "person-sarah-chen", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Sarah Chen works at TechCorp as Senior Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-michael-techcorp", SourceID: "person-michael-brown", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Michael Brown works at TechCorp as Senior ML Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-jennifer-techcorp", SourceID: "person-jennifer-lee", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Jennifer Lee works at TechCorp as Senior Frontend Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-james-techcorp", SourceID: "person-james-taylor", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "James Taylor works at TechCorp as Senior DevOps Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-anna-techcorp", SourceID: "person-anna-garcia", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Anna Garcia works at TechCorp as Software Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-ryan-techcorp", SourceID: "person-ryan-patel", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Ryan Patel works at TechCorp as ML Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-lisa-techcorp", SourceID: "person-lisa-wong", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Lisa Wong works at TechCorp as Frontend Engineer", Strength: 1.0, CreatedAt: now},
		{ID: "rel-mark-techcorp", SourceID: "person-mark-anderson", TargetID: "org-techcorp", Type: "WORKS_AT", Description: "Mark Anderson works at TechCorp as Senior UX Designer", Strength: 1.0, CreatedAt: now},

		// === REPORTS_TO Relationships (Organizational Hierarchy) ===
		{ID: "rel-bob-reports-alice", SourceID: "person-bob-martinez", TargetID: "person-alice-wang", Type: "REPORTS_TO", Description: "Bob Martinez (CTO) reports to Alice Wang (CEO)", Strength: 1.0, CreatedAt: now},
		{ID: "rel-carol-reports-alice", SourceID: "person-carol-johnson", TargetID: "person-alice-wang", Type: "REPORTS_TO", Description: "Carol Johnson (VP Product) reports to Alice Wang (CEO)", Strength: 1.0, CreatedAt: now},
		{ID: "rel-david-reports-bob", SourceID: "person-david-kim", TargetID: "person-bob-martinez", Type: "REPORTS_TO", Description: "David Kim reports to Bob Martinez", Strength: 1.0, CreatedAt: now},
		{ID: "rel-emily-reports-bob", SourceID: "person-emily-chen", TargetID: "person-bob-martinez", Type: "REPORTS_TO", Description: "Emily Chen reports to Bob Martinez", Strength: 1.0, CreatedAt: now},
		{ID: "rel-frank-reports-bob", SourceID: "person-frank-wilson", TargetID: "person-bob-martinez", Type: "REPORTS_TO", Description: "Frank Wilson reports to Bob Martinez", Strength: 1.0, CreatedAt: now},
		{ID: "rel-sarah-reports-david", SourceID: "person-sarah-chen", TargetID: "person-david-kim", Type: "REPORTS_TO", Description: "Sarah Chen reports to David Kim", Strength: 1.0, CreatedAt: now},
		{ID: "rel-james-reports-david", SourceID: "person-james-taylor", TargetID: "person-david-kim", Type: "REPORTS_TO", Description: "James Taylor reports to David Kim", Strength: 1.0, CreatedAt: now},
		{ID: "rel-anna-reports-david", SourceID: "person-anna-garcia", TargetID: "person-david-kim", Type: "REPORTS_TO", Description: "Anna Garcia reports to David Kim", Strength: 1.0, CreatedAt: now},
		{ID: "rel-michael-reports-emily", SourceID: "person-michael-brown", TargetID: "person-emily-chen", Type: "REPORTS_TO", Description: "Michael Brown reports to Emily Chen", Strength: 1.0, CreatedAt: now},
		{ID: "rel-ryan-reports-emily", SourceID: "person-ryan-patel", TargetID: "person-emily-chen", Type: "REPORTS_TO", Description: "Ryan Patel reports to Emily Chen", Strength: 1.0, CreatedAt: now},
		{ID: "rel-jennifer-reports-frank", SourceID: "person-jennifer-lee", TargetID: "person-frank-wilson", Type: "REPORTS_TO", Description: "Jennifer Lee reports to Frank Wilson", Strength: 1.0, CreatedAt: now},
		{ID: "rel-lisa-reports-frank", SourceID: "person-lisa-wong", TargetID: "person-frank-wilson", Type: "REPORTS_TO", Description: "Lisa Wong reports to Frank Wilson", Strength: 1.0, CreatedAt: now},

		// === MANAGES Relationships (Team Management) ===
		{ID: "rel-bob-manages-platform", SourceID: "person-bob-martinez", TargetID: "team-platform", Type: "MANAGES", Description: "Bob Martinez oversees the Platform Team", Strength: 0.8, CreatedAt: now},
		{ID: "rel-bob-manages-aiml", SourceID: "person-bob-martinez", TargetID: "team-aiml", Type: "MANAGES", Description: "Bob Martinez oversees the AI/ML Team", Strength: 0.8, CreatedAt: now},
		{ID: "rel-bob-manages-frontend", SourceID: "person-bob-martinez", TargetID: "team-frontend", Type: "MANAGES", Description: "Bob Martinez oversees the Frontend Team", Strength: 0.8, CreatedAt: now},
		{ID: "rel-david-manages-platform", SourceID: "person-david-kim", TargetID: "team-platform", Type: "MANAGES", Description: "David Kim directly manages the Platform Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-emily-manages-aiml", SourceID: "person-emily-chen", TargetID: "team-aiml", Type: "MANAGES", Description: "Emily Chen directly manages the AI/ML Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-frank-manages-frontend", SourceID: "person-frank-wilson", TargetID: "team-frontend", Type: "MANAGES", Description: "Frank Wilson directly manages the Frontend Team", Strength: 1.0, CreatedAt: now},

		// === PART_OF Relationships (Team Structure) ===
		{ID: "rel-platform-techcorp", SourceID: "team-platform", TargetID: "org-techcorp", Type: "PART_OF", Description: "Platform Team is part of TechCorp Engineering", Strength: 1.0, CreatedAt: now},
		{ID: "rel-aiml-techcorp", SourceID: "team-aiml", TargetID: "org-techcorp", Type: "PART_OF", Description: "AI/ML Team is part of TechCorp Engineering", Strength: 1.0, CreatedAt: now},
		{ID: "rel-frontend-techcorp", SourceID: "team-frontend", TargetID: "org-techcorp", Type: "PART_OF", Description: "Frontend Team is part of TechCorp Engineering", Strength: 1.0, CreatedAt: now},
		{ID: "rel-design-techcorp", SourceID: "team-design", TargetID: "org-techcorp", Type: "PART_OF", Description: "Design Team is part of TechCorp", Strength: 1.0, CreatedAt: now},

		// === MEMBER_OF Relationships (Team Membership) ===
		{ID: "rel-sarah-platform", SourceID: "person-sarah-chen", TargetID: "team-platform", Type: "MEMBER_OF", Description: "Sarah Chen is a member of the Platform Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-james-platform", SourceID: "person-james-taylor", TargetID: "team-platform", Type: "MEMBER_OF", Description: "James Taylor is a member of the Platform Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-anna-platform", SourceID: "person-anna-garcia", TargetID: "team-platform", Type: "MEMBER_OF", Description: "Anna Garcia is a member of the Platform Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-michael-aiml", SourceID: "person-michael-brown", TargetID: "team-aiml", Type: "MEMBER_OF", Description: "Michael Brown is a member of the AI/ML Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-ryan-aiml", SourceID: "person-ryan-patel", TargetID: "team-aiml", Type: "MEMBER_OF", Description: "Ryan Patel is a member of the AI/ML Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-jennifer-frontend", SourceID: "person-jennifer-lee", TargetID: "team-frontend", Type: "MEMBER_OF", Description: "Jennifer Lee is a member of the Frontend Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-lisa-frontend", SourceID: "person-lisa-wong", TargetID: "team-frontend", Type: "MEMBER_OF", Description: "Lisa Wong is a member of the Frontend Team", Strength: 1.0, CreatedAt: now},
		{ID: "rel-mark-design", SourceID: "person-mark-anderson", TargetID: "team-design", Type: "MEMBER_OF", Description: "Mark Anderson is a member of the Design Team", Strength: 1.0, CreatedAt: now},

		// === WORKS_ON Relationships (Project Involvement) ===
		{ID: "rel-david-atlas", SourceID: "person-david-kim", TargetID: "project-atlas", Type: "WORKS_ON", Description: "David Kim leads Project Atlas", Strength: 1.0, CreatedAt: now},
		{ID: "rel-sarah-atlas", SourceID: "person-sarah-chen", TargetID: "project-atlas", Type: "WORKS_ON", Description: "Sarah Chen is a key contributor to Project Atlas", Strength: 0.9, CreatedAt: now},
		{ID: "rel-james-atlas", SourceID: "person-james-taylor", TargetID: "project-atlas", Type: "WORKS_ON", Description: "James Taylor handles DevOps for Project Atlas", Strength: 0.8, CreatedAt: now},
		{ID: "rel-anna-atlas", SourceID: "person-anna-garcia", TargetID: "project-atlas", Type: "WORKS_ON", Description: "Anna Garcia contributes to Project Atlas", Strength: 0.7, CreatedAt: now},
		{ID: "rel-emily-ai-research", SourceID: "person-emily-chen", TargetID: "project-ai-research", Type: "WORKS_ON", Description: "Emily Chen leads the AI Research Initiative", Strength: 1.0, CreatedAt: now},
		{ID: "rel-michael-ai-research", SourceID: "person-michael-brown", TargetID: "project-ai-research", Type: "WORKS_ON", Description: "Michael Brown is lead researcher on AI Research", Strength: 0.95, CreatedAt: now},
		{ID: "rel-ryan-ai-research", SourceID: "person-ryan-patel", TargetID: "project-ai-research", Type: "WORKS_ON", Description: "Ryan Patel works on computer vision for AI Research", Strength: 0.8, CreatedAt: now},
		{ID: "rel-frank-dashboard", SourceID: "person-frank-wilson", TargetID: "project-dashboard", Type: "WORKS_ON", Description: "Frank Wilson leads the Analytics Dashboard project", Strength: 1.0, CreatedAt: now},
		{ID: "rel-jennifer-dashboard", SourceID: "person-jennifer-lee", TargetID: "project-dashboard", Type: "WORKS_ON", Description: "Jennifer Lee is lead frontend dev on Dashboard", Strength: 0.9, CreatedAt: now},
		{ID: "rel-lisa-dashboard", SourceID: "person-lisa-wong", TargetID: "project-dashboard", Type: "WORKS_ON", Description: "Lisa Wong works on Dashboard components", Strength: 0.7, CreatedAt: now},
		{ID: "rel-mark-dashboard", SourceID: "person-mark-anderson", TargetID: "project-dashboard", Type: "WORKS_ON", Description: "Mark Anderson designs the Dashboard UX", Strength: 0.8, CreatedAt: now},
		{ID: "rel-carol-mobile", SourceID: "person-carol-johnson", TargetID: "project-mobile-app", Type: "WORKS_ON", Description: "Carol Johnson sponsors the Mobile App project", Strength: 0.6, CreatedAt: now},

		// === LEADS Relationships (Project Leadership) ===
		{ID: "rel-david-leads-atlas", SourceID: "person-david-kim", TargetID: "project-atlas", Type: "LEADS", Description: "David Kim is the technical lead for Project Atlas", Strength: 1.0, CreatedAt: now},
		{ID: "rel-emily-leads-ai", SourceID: "person-emily-chen", TargetID: "project-ai-research", Type: "LEADS", Description: "Emily Chen is the technical lead for AI Research", Strength: 1.0, CreatedAt: now},
		{ID: "rel-frank-leads-dashboard", SourceID: "person-frank-wilson", TargetID: "project-dashboard", Type: "LEADS", Description: "Frank Wilson is the technical lead for Dashboard", Strength: 1.0, CreatedAt: now},

		// === USES Relationships (Technology Usage) ===
		{ID: "rel-atlas-go", SourceID: "project-atlas", TargetID: "tech-go", Type: "USES", Description: "Project Atlas is built with Go", Strength: 1.0, CreatedAt: now},
		{ID: "rel-atlas-k8s", SourceID: "project-atlas", TargetID: "tech-kubernetes", Type: "USES", Description: "Project Atlas deploys on Kubernetes", Strength: 0.9, CreatedAt: now},
		{ID: "rel-atlas-postgres", SourceID: "project-atlas", TargetID: "tech-postgresql", Type: "USES", Description: "Project Atlas uses PostgreSQL", Strength: 0.8, CreatedAt: now},
		{ID: "rel-ai-python", SourceID: "project-ai-research", TargetID: "tech-python", Type: "USES", Description: "AI Research uses Python", Strength: 1.0, CreatedAt: now},
		{ID: "rel-ai-pytorch", SourceID: "project-ai-research", TargetID: "tech-pytorch", Type: "USES", Description: "AI Research uses PyTorch", Strength: 0.95, CreatedAt: now},
		{ID: "rel-dashboard-ts", SourceID: "project-dashboard", TargetID: "tech-typescript", Type: "USES", Description: "Dashboard is built with TypeScript", Strength: 1.0, CreatedAt: now},
		{ID: "rel-dashboard-react", SourceID: "project-dashboard", TargetID: "tech-react", Type: "USES", Description: "Dashboard uses React", Strength: 1.0, CreatedAt: now},

		// === COLLABORATES_WITH Relationships ===
		{ID: "rel-sarah-james-collab", SourceID: "person-sarah-chen", TargetID: "person-james-taylor", Type: "COLLABORATES_WITH", Description: "Sarah and James collaborate on infrastructure", Strength: 0.9, CreatedAt: now},
		{ID: "rel-sarah-anna-collab", SourceID: "person-sarah-chen", TargetID: "person-anna-garcia", Type: "COLLABORATES_WITH", Description: "Sarah mentors Anna on backend development", Strength: 0.8, CreatedAt: now},
		{ID: "rel-michael-ryan-collab", SourceID: "person-michael-brown", TargetID: "person-ryan-patel", Type: "COLLABORATES_WITH", Description: "Michael and Ryan collaborate on ML models", Strength: 0.85, CreatedAt: now},
		{ID: "rel-jennifer-lisa-collab", SourceID: "person-jennifer-lee", TargetID: "person-lisa-wong", Type: "COLLABORATES_WITH", Description: "Jennifer and Lisa pair program on frontend", Strength: 0.9, CreatedAt: now},
		{ID: "rel-jennifer-mark-collab", SourceID: "person-jennifer-lee", TargetID: "person-mark-anderson", Type: "COLLABORATES_WITH", Description: "Jennifer works with Mark on UI implementation", Strength: 0.8, CreatedAt: now},
		{ID: "rel-sarah-michael-collab", SourceID: "person-sarah-chen", TargetID: "person-michael-brown", Type: "COLLABORATES_WITH", Description: "Sarah and Michael integrate ML services with platform", Strength: 0.7, CreatedAt: now},

		// === PARTNERS_WITH Relationships (Organizational) ===
		{ID: "rel-techcorp-ailabs", SourceID: "org-techcorp", TargetID: "org-ai-labs", Type: "PARTNERS_WITH", Description: "TechCorp partners with AI Labs for research", Strength: 0.9, CreatedAt: now},
		{ID: "rel-techcorp-cloud", SourceID: "org-techcorp", TargetID: "org-cloudservices", Type: "PARTNERS_WITH", Description: "TechCorp uses CloudServices for infrastructure", Strength: 0.85, CreatedAt: now},

		// === CITY_IN Relationships (City → Country) ===
		{ID: "rel-sf-usa", SourceID: "city-san-francisco", TargetID: "country-usa", Type: "CITY_IN", Description: "San Francisco is located in the United States", Strength: 1.0, CreatedAt: now},
		{ID: "rel-nyc-usa", SourceID: "city-new-york", TargetID: "country-usa", Type: "CITY_IN", Description: "New York is located in the United States", Strength: 1.0, CreatedAt: now},
		{ID: "rel-boston-usa", SourceID: "city-boston", TargetID: "country-usa", Type: "CITY_IN", Description: "Boston is located in the United States", Strength: 1.0, CreatedAt: now},
		{ID: "rel-london-uk", SourceID: "city-london", TargetID: "country-uk", Type: "CITY_IN", Description: "London is located in the United Kingdom", Strength: 1.0, CreatedAt: now},
		{ID: "rel-berlin-germany", SourceID: "city-berlin", TargetID: "country-germany", Type: "CITY_IN", Description: "Berlin is located in Germany", Strength: 1.0, CreatedAt: now},
		{ID: "rel-tokyo-japan", SourceID: "city-tokyo", TargetID: "country-japan", Type: "CITY_IN", Description: "Tokyo is located in Japan", Strength: 1.0, CreatedAt: now},

		// === HEADQUARTERED_IN Relationships (Organization → City) ===
		{ID: "rel-techcorp-hq-sf", SourceID: "org-techcorp", TargetID: "city-san-francisco", Type: "HEADQUARTERED_IN", Description: "TechCorp is headquartered in San Francisco", Strength: 1.0, CreatedAt: now},
		{ID: "rel-ailabs-hq-boston", SourceID: "org-ai-labs", TargetID: "city-boston", Type: "HEADQUARTERED_IN", Description: "AI Labs Inc is headquartered in Boston near MIT", Strength: 1.0, CreatedAt: now},
		{ID: "rel-cloud-hq-nyc", SourceID: "org-cloudservices", TargetID: "city-new-york", Type: "HEADQUARTERED_IN", Description: "CloudServices Co is headquartered in New York", Strength: 1.0, CreatedAt: now},

		// === HAS_OFFICE_IN Relationships (Organization → City) ===
		{ID: "rel-techcorp-office-nyc", SourceID: "org-techcorp", TargetID: "city-new-york", Type: "HAS_OFFICE_IN", Description: "TechCorp has an office in New York", Strength: 0.8, CreatedAt: now},
		{ID: "rel-techcorp-office-london", SourceID: "org-techcorp", TargetID: "city-london", Type: "HAS_OFFICE_IN", Description: "TechCorp has a European office in London", Strength: 0.7, CreatedAt: now},
		{ID: "rel-techcorp-office-tokyo", SourceID: "org-techcorp", TargetID: "city-tokyo", Type: "HAS_OFFICE_IN", Description: "TechCorp has an Asian office in Tokyo", Strength: 0.6, CreatedAt: now},
		{ID: "rel-cloud-office-sf", SourceID: "org-cloudservices", TargetID: "city-san-francisco", Type: "HAS_OFFICE_IN", Description: "CloudServices Co has an office in San Francisco", Strength: 0.7, CreatedAt: now},
		{ID: "rel-cloud-office-berlin", SourceID: "org-cloudservices", TargetID: "city-berlin", Type: "HAS_OFFICE_IN", Description: "CloudServices Co has a European office in Berlin", Strength: 0.6, CreatedAt: now},

		// === BASED_IN Relationships (Person → City) ===
		{ID: "rel-alice-sf", SourceID: "person-alice-wang", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "Alice Wang is based in San Francisco (HQ)", Strength: 1.0, CreatedAt: now},
		{ID: "rel-bob-sf", SourceID: "person-bob-martinez", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "Bob Martinez is based in San Francisco (HQ)", Strength: 1.0, CreatedAt: now},
		{ID: "rel-carol-sf", SourceID: "person-carol-johnson", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "Carol Johnson is based in San Francisco", Strength: 1.0, CreatedAt: now},
		{ID: "rel-david-sf", SourceID: "person-david-kim", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "David Kim is based in San Francisco", Strength: 1.0, CreatedAt: now},
		{ID: "rel-emily-boston", SourceID: "person-emily-chen", TargetID: "city-boston", Type: "BASED_IN", Description: "Emily Chen is based in Boston, works closely with AI Labs", Strength: 1.0, CreatedAt: now},
		{ID: "rel-frank-nyc", SourceID: "person-frank-wilson", TargetID: "city-new-york", Type: "BASED_IN", Description: "Frank Wilson is based in New York office", Strength: 1.0, CreatedAt: now},
		{ID: "rel-sarah-sf", SourceID: "person-sarah-chen", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "Sarah Chen is based in San Francisco", Strength: 1.0, CreatedAt: now},
		{ID: "rel-michael-boston", SourceID: "person-michael-brown", TargetID: "city-boston", Type: "BASED_IN", Description: "Michael Brown is based in Boston", Strength: 1.0, CreatedAt: now},
		{ID: "rel-jennifer-nyc", SourceID: "person-jennifer-lee", TargetID: "city-new-york", Type: "BASED_IN", Description: "Jennifer Lee is based in New York", Strength: 1.0, CreatedAt: now},
		{ID: "rel-james-sf", SourceID: "person-james-taylor", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "James Taylor is based in San Francisco", Strength: 1.0, CreatedAt: now},
		{ID: "rel-anna-sf", SourceID: "person-anna-garcia", TargetID: "city-san-francisco", Type: "BASED_IN", Description: "Anna Garcia is based in San Francisco", Strength: 1.0, CreatedAt: now},
		{ID: "rel-ryan-boston", SourceID: "person-ryan-patel", TargetID: "city-boston", Type: "BASED_IN", Description: "Ryan Patel is based in Boston", Strength: 1.0, CreatedAt: now},
		{ID: "rel-lisa-nyc", SourceID: "person-lisa-wong", TargetID: "city-new-york", Type: "BASED_IN", Description: "Lisa Wong is based in New York", Strength: 1.0, CreatedAt: now},
		{ID: "rel-mark-london", SourceID: "person-mark-anderson", TargetID: "city-london", Type: "BASED_IN", Description: "Mark Anderson is based in London office", Strength: 1.0, CreatedAt: now},
	}
}
