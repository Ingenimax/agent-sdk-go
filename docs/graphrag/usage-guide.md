# GraphRAG Usage Guide

This guide covers how to use GraphRAG in the Agent SDK to build knowledge graph-powered agents.

## Prerequisites

- **Weaviate**: A running Weaviate instance (local or cloud)
- **API Keys**: Anthropic, OpenAI, or another supported LLM provider

### Starting Weaviate Locally

```bash
docker run -d \
  --name weaviate \
  -p 8080:8080 \
  -e QUERY_DEFAULTS_LIMIT=25 \
  -e AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED=true \
  -e PERSISTENCE_DATA_PATH=/var/lib/weaviate \
  -e DEFAULT_VECTORIZER_MODULE=none \
  -e CLUSTER_HOSTNAME=node1 \
  cr.weaviate.io/semitechnologies/weaviate:1.24.1
```

## Quick Start

### 1. Create a GraphRAG Store

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/graphrag/weaviate"
)

// Create store (no embedder - uses keyword search)
store, err := weaviate.New(&weaviate.Config{
    Host:        "localhost:8080",
    Scheme:      "http",
    ClassPrefix: "MyApp",  // Creates MyAppEntity and MyAppRelationship collections
})
if err != nil {
    log.Fatalf("Failed to create store: %v", err)
}
defer store.Close()
```

### 2. With Embeddings (Recommended)

For semantic search, configure an embedder:

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/graphrag/weaviate"
    "github.com/Ingenimax/agent-sdk-go/pkg/embedding/openai"
)

// Create embedder
embedder := openai.NewEmbedder(os.Getenv("OPENAI_API_KEY"))

// Create store with embedder
store, err := weaviate.New(&weaviate.Config{
    Host:        "localhost:8080",
    Scheme:      "http",
    ClassPrefix: "MyApp",
}, weaviate.WithEmbedder(embedder))
```

### 3. Create an Agent with GraphRAG

```go
import (
    "github.com/Ingenimax/agent-sdk-go/pkg/agent"
    "github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

// Create LLM
llm := anthropic.NewClient(
    os.Getenv("ANTHROPIC_API_KEY"),
    anthropic.WithModel("claude-sonnet-4-20250514"),
)

// Create agent with GraphRAG
ag, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithGraphRAG(store),  // Automatically registers 5 GraphRAG tools
    agent.WithName("KnowledgeAgent"),
    agent.WithSystemPrompt(`You have access to a knowledge graph. Use graphrag_search to find information.`),
)
if err != nil {
    log.Fatalf("Failed to create agent: %v", err)
}

// Query the agent
response, err := ag.Run(ctx, "Who works on the Platform team?")
```

## Working with Entities

### Storing Entities

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

entities := []interfaces.Entity{
    {
        ID:          "person-john",
        Name:        "John Smith",
        Type:        "Person",
        Description: "Senior software engineer, Go expert, leads the Platform team",
        Properties: map[string]interface{}{
            "role":       "Senior Engineer",
            "department": "Engineering",
            "skills":     []string{"Go", "Kubernetes", "PostgreSQL"},
        },
    },
    {
        ID:          "project-atlas",
        Name:        "Project Atlas",
        Type:        "Project",
        Description: "Core platform modernization initiative migrating to microservices",
        Properties: map[string]interface{}{
            "status":   "Active",
            "priority": "High",
        },
    },
    {
        ID:          "org-techcorp",
        Name:        "TechCorp",
        Type:        "Organization",
        Description: "Technology company specializing in enterprise solutions",
        Properties: map[string]interface{}{
            "industry": "Technology",
            "size":     "500 employees",
        },
    },
}

err := store.StoreEntities(ctx, entities)
if err != nil {
    log.Fatalf("Failed to store entities: %v", err)
}
```

### Retrieving Entities

```go
// Get by ID
entity, err := store.GetEntity(ctx, "person-john")
if err != nil {
    log.Fatalf("Failed to get entity: %v", err)
}
fmt.Printf("Name: %s, Type: %s\n", entity.Name, entity.Type)

// Access properties
if role, ok := entity.Properties["role"].(string); ok {
    fmt.Printf("Role: %s\n", role)
}
```

### Updating Entities

```go
entity, _ := store.GetEntity(ctx, "person-john")
entity.Description = "Senior software engineer and Tech Lead"
entity.Properties["role"] = "Tech Lead"

err := store.UpdateEntity(ctx, *entity)
```

### Deleting Entities

```go
err := store.DeleteEntity(ctx, "person-john")
```

## Working with Relationships

### Storing Relationships

```go
relationships := []interfaces.Relationship{
    {
        ID:          "rel-john-techcorp",
        SourceID:    "person-john",
        TargetID:    "org-techcorp",
        Type:        "WORKS_AT",
        Description: "John Smith works at TechCorp as Senior Engineer",
        Strength:    1.0,
    },
    {
        ID:          "rel-john-atlas",
        SourceID:    "person-john",
        TargetID:    "project-atlas",
        Type:        "LEADS",
        Description: "John leads Project Atlas",
        Strength:    1.0,
    },
    {
        ID:          "rel-john-manages-alice",
        SourceID:    "person-john",
        TargetID:    "person-alice",
        Type:        "MANAGES",
        Description: "John manages Alice on the Platform team",
        Strength:    0.9,
    },
}

err := store.StoreRelationships(ctx, relationships)
```

### Common Relationship Types

| Type | Description | Example |
|------|-------------|---------|
| `WORKS_AT` | Person employed by organization | John → TechCorp |
| `WORKS_ON` | Person contributes to project | John → Project Atlas |
| `MANAGES` | Person manages another person | John → Alice |
| `REPORTS_TO` | Person reports to another | Alice → John |
| `LEADS` | Person leads project/team | John → Platform Team |
| `MEMBER_OF` | Person belongs to team | Alice → Platform Team |
| `PART_OF` | Team/dept part of organization | Platform Team → TechCorp |
| `USES` | Project uses technology | Project Atlas → Go |
| `COLLABORATES_WITH` | People work together | John → Bob |
| `PARTNERS_WITH` | Organizations partner | TechCorp → AI Labs |
| `HEADQUARTERED_IN` | Organization HQ location | TechCorp → San Francisco |
| `BASED_IN` | Person's work location | John → San Francisco |
| `CITY_IN` | City in country | San Francisco → USA |

### Retrieving Relationships

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/graphrag"

// Get outgoing relationships (from entity)
outgoing, err := store.GetRelationships(ctx, "person-john", graphrag.DirectionOutgoing)
for _, rel := range outgoing {
    fmt.Printf("%s -[%s]-> %s\n", rel.SourceID, rel.Type, rel.TargetID)
}

// Get incoming relationships (to entity)
incoming, err := store.GetRelationships(ctx, "project-atlas", graphrag.DirectionIncoming)

// Get both directions
all, err := store.GetRelationships(ctx, "person-john", graphrag.DirectionBoth)
```

### Deleting Relationships

```go
err := store.DeleteRelationship(ctx, "rel-john-atlas")
```

## Searching the Knowledge Graph

### Basic Search (Hybrid)

```go
results, err := store.Search(ctx, "senior engineers", 10)
for _, r := range results {
    fmt.Printf("Entity: %s (Score: %.2f)\n", r.Entity.Name, r.Score)
}
```

### Search with Options

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/graphrag"

// Hybrid search (vector + keyword)
results, err := store.Search(ctx, "platform infrastructure", 10,
    graphrag.WithMode(graphrag.SearchModeHybrid),
)

// Vector-only search (semantic similarity)
results, err := store.Search(ctx, "engineers who build APIs", 10,
    graphrag.WithMode(graphrag.SearchModeVector),
)

// Keyword-only search (exact matches)
results, err := store.Search(ctx, "Platform Team", 10,
    graphrag.WithMode(graphrag.SearchModeKeyword),
)

// Filter by entity types
results, err := store.Search(ctx, "team leads", 10,
    graphrag.WithEntityTypes("Person"),
)

// Include relationships in results
results, err := store.Search(ctx, "project managers", 10,
    graphrag.WithIncludeRelationships(true),
)
```

### Local Search (Entity-Focused)

Local search finds entities matching the query and expands context through graph traversal:

```go
// Search with 2-hop graph traversal
results, err := store.LocalSearch(ctx, "John's projects", "", 2)

// Search from a specific entity
results, err := store.LocalSearch(ctx, "projects", "person-john", 2)

for _, r := range results {
    fmt.Printf("Entity: %s\n", r.Entity.Name)
    fmt.Printf("Context entities: %d\n", len(r.Context))
    for _, ctx := range r.Context {
        fmt.Printf("  - %s (%s)\n", ctx.Name, ctx.Type)
    }
}
```

### Global Search (Community-Based)

Global search retrieves information across entity type communities:

```go
results, err := store.GlobalSearch(ctx, "team structure", 1)

for _, r := range results {
    fmt.Printf("Community: %s\n", r.CommunityID)  // e.g., "Person", "Team"
    fmt.Printf("Entity: %s\n", r.Entity.Name)
}
```

## Graph Traversal

### Traverse from Entity

Get all entities and relationships within N hops:

```go
context, err := store.TraverseFrom(ctx, "person-john", 2)

fmt.Printf("Central: %s\n", context.CentralEntity.Name)
fmt.Printf("Found %d entities, %d relationships\n",
    len(context.Entities), len(context.Relationships))

// Print the subgraph
for _, entity := range context.Entities {
    fmt.Printf("  Entity: %s (%s)\n", entity.Name, entity.Type)
}
for _, rel := range context.Relationships {
    fmt.Printf("  %s -[%s]-> %s\n", rel.SourceID, rel.Type, rel.TargetID)
}
```

### Find Shortest Path

```go
path, err := store.ShortestPath(ctx, "person-john", "project-dashboard")

if path != nil {
    fmt.Printf("Path length: %d hops\n", path.Length)
    fmt.Printf("From: %s\n", path.Source.Name)
    for i, entity := range path.Entities {
        fmt.Printf("  -[%s]-> %s\n", path.Relationships[i].Type, entity.Name)
    }
    fmt.Printf("  -[%s]-> %s\n",
        path.Relationships[len(path.Relationships)-1].Type,
        path.Target.Name)
}
```

## Entity Extraction from Text

Extract entities and relationships from unstructured text using LLM:

```go
text := `
John Smith joined TechCorp as a Senior Engineer last month.
He now leads Project Atlas and works closely with Alice Johnson.
The project uses Go and Kubernetes for the backend infrastructure.
`

result, err := store.ExtractFromText(ctx, text, llm)

fmt.Printf("Extracted %d entities:\n", len(result.Entities))
for _, e := range result.Entities {
    fmt.Printf("  - %s (%s): %s\n", e.Name, e.Type, e.Description)
}

fmt.Printf("Extracted %d relationships:\n", len(result.Relationships))
for _, r := range result.Relationships {
    fmt.Printf("  - %s -[%s]-> %s\n", r.SourceID, r.Type, r.TargetID)
}

// Store extracted data
if err := store.StoreEntities(ctx, result.Entities); err != nil {
    log.Printf("Failed to store entities: %v", err)
}
if err := store.StoreRelationships(ctx, result.Relationships); err != nil {
    log.Printf("Failed to store relationships: %v", err)
}
```

### Schema-Guided Extraction

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/graphrag"

result, err := store.ExtractFromText(ctx, text, llm,
    graphrag.WithSchemaGuidance(true),
    graphrag.WithEntityTypes("Person", "Project", "Technology"),
    graphrag.WithRelationshipTypes("WORKS_ON", "LEADS", "USES"),
)
```

## Multi-Tenancy

GraphRAG supports organization-based data isolation:

### Context-Based (Preferred)

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"

// Create context with organization ID
ctx := multitenancy.WithOrgID(context.Background(), "org-123")

// All operations are now scoped to this tenant
err := store.StoreEntities(ctx, entities)
results, err := store.Search(ctx, "engineers")
```

### Store-Level

```go
// Set tenant for all operations
store.SetTenant("org-123")

// Operations use this tenant
err := store.StoreEntities(ctx, entities)

// Clear tenant
store.SetTenant("")
```

### Per-Operation

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/graphrag"

err := store.StoreEntities(ctx, entities, graphrag.WithTenant("org-123"))
results, err := store.Search(ctx, "query", 10, graphrag.WithTenant("org-123"))
```

## Schema Discovery

Discover entity and relationship types from existing data:

```go
schema, err := store.DiscoverSchema(ctx)

fmt.Println("Entity Types:")
for _, et := range schema.EntityTypes {
    fmt.Printf("  - %s: %s\n", et.Name, et.Description)
}

fmt.Println("Relationship Types:")
for _, rt := range schema.RelationshipTypes {
    fmt.Printf("  - %s: %v -> %v\n", rt.Name, rt.SourceTypes, rt.TargetTypes)
}
```

## Built-in Agent Tools

When you use `agent.WithGraphRAG(store)`, five tools are automatically available:

### graphrag_search

Search the knowledge graph.

**Parameters:**
- `query` (string, required): Search query
- `search_type` (string): "local", "global", or "hybrid" (default)
- `limit` (number): Max results (default: 10)
- `entity_types` (array): Filter by types, e.g., ["Person", "Project"]
- `depth` (number): Traversal depth for local search (default: 2)

**Example agent conversation:**
```
User: "Find information about the Platform team"
Agent: [Uses graphrag_search with query="Platform team"]
```

### graphrag_add_entity

Add a new entity to the graph.

**Parameters:**
- `name` (string, required): Entity name
- `type` (string, required): Entity type
- `description` (string): Entity description
- `properties` (object): Additional metadata

**Example:**
```
User: "Add Sarah Chen as a new designer"
Agent: [Uses graphrag_add_entity with name="Sarah Chen", type="Person",
        properties={"role": "Designer", "department": "Design"}]
```

### graphrag_add_relationship

Create a relationship between entities.

**Parameters:**
- `source_id` (string, required): Source entity ID
- `target_id` (string, required): Target entity ID
- `type` (string, required): Relationship type
- `description` (string): Relationship description
- `strength` (number): 0.0 to 1.0

**Example:**
```
User: "John now manages Sarah"
Agent: [Uses graphrag_add_relationship with source_id="person-john",
        target_id="person-sarah", type="MANAGES"]
```

### graphrag_get_context

Get entity context through graph traversal.

**Parameters:**
- `entity_id` (string, required): Entity ID
- `depth` (number): Traversal depth (default: 2)

**Example:**
```
User: "Tell me everything about John and his connections"
Agent: [Uses graphrag_get_context with entity_id="person-john", depth=2]
```

### graphrag_extract

Extract entities and relationships from text.

**Parameters:**
- `text` (string, required): Text to extract from
- `schema_guided` (boolean): Use schema guidance (default: true)

**Example:**
```
User: "Extract entities from: 'Alice works with Bob on the API project'"
Agent: [Uses graphrag_extract with text="Alice works with Bob on the API project"]
```

## Complete Example

See `examples/graphrag-agent/main.go` for a complete working example with:

- 14 People (executives, managers, engineers, designers)
- 3 Organizations
- 4 Teams
- 5 Projects
- 7 Technologies
- 4 Countries
- 6 Cities
- 110+ Relationships

Run the example:

```bash
# Start Weaviate
docker run -d -p 8080:8080 cr.weaviate.io/semitechnologies/weaviate:1.24.1

# Set API key
export ANTHROPIC_API_KEY=your-key

# Run example
cd examples/graphrag-agent
go run main.go
```

Sample questions the agent can answer:
- "Who is the CTO and what teams do they manage?"
- "What projects is the Platform team working on?"
- "Who are the senior engineers and what technologies do they use?"
- "What organizations are located in San Francisco?"
- "Which cities in the USA have tech companies?"

## GraphRAG as Agent Memory

One powerful use case is using GraphRAG as **persistent structured memory** for agents. Unlike simple chat history or key-value storage, graph memory lets agents:

- **Learn**: Store facts as entities and relationships
- **Recall**: Search and traverse to answer questions
- **Connect**: Build knowledge incrementally across conversations
- **Reason**: Follow relationship paths to derive insights

### Memory Agent Pattern

```go
// Create store with user-specific tenant for memory isolation
ctx := multitenancy.WithOrgID(ctx, userID)

store, _ := weaviate.New(&weaviate.Config{
    Host:        "localhost:8080",
    Scheme:      "http",
    ClassPrefix: "Memory",
}, weaviate.WithStoreTenant(userID))

// Create memory-enabled agent
ag, _ := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithGraphRAG(store),
    agent.WithSystemPrompt(memoryAgentPrompt),
)
```

### Memory Agent System Prompt

Guide the agent on how to use its memory:

```
You are an assistant with persistent memory. When users tell you things:
1. Extract key entities (people, projects, skills, locations)
2. Store them with graphrag_add_entity
3. Create relationships with graphrag_add_relationship

When users ask questions:
1. Search memory with graphrag_search
2. Explore context with graphrag_get_context
3. Synthesize information naturally

Entity types: Person, Organization, Project, Skill, Location, Topic
Relationship types: WORKS_AT, WORKS_ON, KNOWS, USES, INTERESTED_IN
```

### Example Conversation

```
User: "I'm Alex, I work at TechCorp as a senior engineer"

Agent: [Internally calls:]
  - graphrag_add_entity: {id: "person-alex", name: "Alex", type: "Person"}
  - graphrag_add_entity: {id: "org-techcorp", name: "TechCorp", type: "Organization"}
  - graphrag_add_relationship: {source: "person-alex", target: "org-techcorp", type: "WORKS_AT"}

"Nice to meet you, Alex! I've noted that you're a senior engineer at TechCorp."

User: "What do you know about me?"

Agent: [Calls graphrag_search with query="Alex"]

"You're Alex, a senior engineer at TechCorp. Is there anything else you'd like to tell me?"
```

### Memory Isolation with Multi-Tenancy

Each user can have isolated memory space:

```go
// User A's memory
ctxA := multitenancy.WithOrgID(ctx, "user-alice")

// User B's memory (completely separate)
ctxB := multitenancy.WithOrgID(ctx, "user-bob")
```

### Complete Memory Agent Example

See `examples/graphrag-memory-agent/main.go` for an interactive demo:

```bash
export ANTHROPIC_API_KEY=your-key
export OPENAI_API_KEY=your-key  # Optional: for semantic search
cd examples/graphrag-memory-agent
go run main.go
```

Features:
- Interactive conversation with memory
- `memory` command to inspect stored knowledge
- `clear` command to reset memory
- Per-user memory isolation via `USER_ID` environment variable

## Best Practices

### 1. Use Meaningful Entity IDs

```go
// Good - descriptive and stable
entity := interfaces.Entity{ID: "person-john-smith", ...}
entity := interfaces.Entity{ID: "project-atlas-2024", ...}
entity := interfaces.Entity{ID: "org-techcorp", ...}

// Avoid - non-descriptive
entity := interfaces.Entity{ID: "12345", ...}
entity := interfaces.Entity{ID: uuid.New().String(), ...}  // Hard to reference
```

### 2. Write Rich Descriptions

Descriptions are used for embeddings and semantic search:

```go
// Good - detailed description
entity := interfaces.Entity{
    Name: "Project Atlas",
    Description: "Core platform modernization initiative upgrading infrastructure " +
                 "to microservices architecture using Go, Kubernetes, and PostgreSQL. " +
                 "Led by Platform team. Expected completion Q2 2025.",
}

// Avoid - minimal description
entity := interfaces.Entity{
    Name: "Project Atlas",
    Description: "A project",
}
```

### 3. Use Relationship Strength Appropriately

```go
// Primary responsibility (1.0)
interfaces.Relationship{Type: "LEADS", Strength: 1.0}

// Regular involvement (0.7-0.9)
interfaces.Relationship{Type: "WORKS_ON", Strength: 0.8}

// Occasional involvement (0.3-0.6)
interfaces.Relationship{Type: "CONTRIBUTES_TO", Strength: 0.4}
```

### 4. Choose the Right Search Mode

| Query Type | Mode | Example |
|------------|------|---------|
| Specific entity | LocalSearch | "What does John work on?" |
| Summary/overview | GlobalSearch | "Team structure overview" |
| General info | Hybrid (default) | "Tell me about the platform" |
| Exact matches | Keyword | "Find 'Platform Team'" |

### 5. Keep Traversal Depth Reasonable

```go
// Good - interactive queries
context, _ := store.TraverseFrom(ctx, "person-john", 2)

// Avoid - too slow with Weaviate
context, _ := store.TraverseFrom(ctx, "person-john", 5)
```

### 6. Use Multi-Tenancy for Data Isolation

Always set tenant context for multi-tenant applications:

```go
ctx := multitenancy.WithOrgID(context.Background(), userOrgID)
// All operations are now isolated to this organization
```

## Troubleshooting

### No Search Results

1. **Verify entities exist:**
   ```go
   entity, err := store.GetEntity(ctx, "expected-id")
   ```

2. **Check tenant context matches:**
   ```go
   // Entities stored with tenant "org-123"
   ctx := multitenancy.WithOrgID(ctx, "org-123")
   results, _ := store.Search(ctx, "query", 10)
   ```

3. **Without embedder, use keyword search:**
   ```go
   results, _ := store.Search(ctx, "exact term", 10,
       graphrag.WithMode(graphrag.SearchModeKeyword),
   )
   ```

### Hybrid Search Returns Empty

Hybrid search requires an embedder. Without one, it falls back to keyword search automatically. Ensure embedder is configured:

```go
store, _ := weaviate.New(config, weaviate.WithEmbedder(embedder))
```

### Slow Performance

1. Reduce traversal depth to <= 2
2. Use more specific search queries
3. Limit results with smaller `limit` values
4. Filter by entity types to narrow results

### Connection Issues

1. Verify Weaviate is running: `curl http://localhost:8080/v1/.well-known/ready`
2. Check host/scheme configuration
3. For cloud instances, ensure API key is set

## Cleanup

Delete all GraphRAG collections:

```go
err := store.DeleteSchema(ctx)
if err != nil {
    log.Printf("Failed to delete schema: %v", err)
}
```

Always close the store when done:

```go
defer store.Close()
```
