# GraphRAG Architecture

This document describes the architecture and implementation of GraphRAG in the Agent SDK.

## Overview

GraphRAG (Graph-based Retrieval-Augmented Generation) extends traditional RAG by leveraging knowledge graphs for enhanced context retrieval. Unlike pure vector search, GraphRAG maintains explicit relationships between entities, enabling:

- **Relationship-aware retrieval**: Find not just similar entities, but their connections
- **Multi-hop graph traversal**: Explore entity neighborhoods with configurable depth
- **Local and global search**: Entity-focused or community-wide queries
- **Entity extraction**: Automatically extract entities and relationships from text using LLM

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              Agent                                       │
│  agent.NewAgent(                                                         │
│      agent.WithGraphRAG(store),  ← Enables 5 GraphRAG tools             │
│      agent.WithLLM(llm),                                                 │
│  )                                                                       │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    pkg/tools/graphrag/                                   │
│  ┌─────────────┐ ┌───────────────┐ ┌──────────────────┐                │
│  │ search.go   │ │ add_entity.go │ │ add_relationship │                │
│  │             │ │               │ │      .go         │                │
│  └─────────────┘ └───────────────┘ └──────────────────┘                │
│  ┌─────────────────┐ ┌─────────────┐                                    │
│  │ get_context.go  │ │ extract.go  │                                    │
│  └─────────────────┘ └─────────────┘                                    │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    pkg/interfaces/graphrag.go                            │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                     GraphRAGStore Interface                        │  │
│  │  • StoreEntities(entities)      • GetEntity(id)                   │  │
│  │  • StoreRelationships(rels)     • GetRelationships(entityID)      │  │
│  │  • Search(query, limit)         • LocalSearch(query, entityID)    │  │
│  │  • GlobalSearch(query, level)   • TraverseFrom(entityID, depth)   │  │
│  │  • ShortestPath(source, target) • ExtractFromText(text, llm)      │  │
│  │  • DiscoverSchema()             • DeleteSchema()                   │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    pkg/graphrag/weaviate/                                │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    Weaviate Implementation                       │    │
│  │  store.go       - Main store, config, lifecycle                  │    │
│  │  schema.go      - Collection creation, schema management         │    │
│  │  entity.go      - Entity CRUD operations                         │    │
│  │  relationship.go - Relationship CRUD operations                  │    │
│  │  search.go      - Vector, keyword, hybrid search                 │    │
│  │  traversal.go   - BFS graph traversal, shortest path             │    │
│  │  extraction.go  - LLM-based entity extraction                    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         Weaviate Database                                │
│  ┌──────────────────────────┐  ┌──────────────────────────┐            │
│  │  {Prefix}Entity          │  │  {Prefix}Relationship    │            │
│  │  ─────────────────────   │  │  ─────────────────────   │            │
│  │  entityId    (text)      │  │  relationshipId (text)   │            │
│  │  name        (text)      │  │  sourceId      (text)    │            │
│  │  entityType  (text)      │  │  targetId      (text)    │            │
│  │  description (text)      │  │  relationshipType (text) │            │
│  │  properties  (text/JSON) │  │  description   (text)    │            │
│  │  orgId       (text)      │  │  strength     (number)   │            │
│  │  createdAt   (date)      │  │  properties   (text)     │            │
│  │  updatedAt   (date)      │  │  orgId        (text)     │            │
│  │  [vector]                │  │  createdAt    (date)     │            │
│  └──────────────────────────┘  └──────────────────────────┘            │
└─────────────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. GraphRAGStore Interface

The `GraphRAGStore` interface (`pkg/interfaces/graphrag.go`) defines all operations:

```go
type GraphRAGStore interface {
    // Entity operations
    StoreEntities(ctx context.Context, entities []Entity, opts ...GraphStoreOption) error
    GetEntity(ctx context.Context, id string, opts ...GraphStoreOption) (*Entity, error)
    UpdateEntity(ctx context.Context, entity Entity, opts ...GraphStoreOption) error
    DeleteEntity(ctx context.Context, id string, opts ...GraphStoreOption) error

    // Relationship operations
    StoreRelationships(ctx context.Context, relationships []Relationship, opts ...GraphStoreOption) error
    GetRelationships(ctx context.Context, entityID string, direction RelationshipDirection, opts ...GraphStoreOption) ([]Relationship, error)
    DeleteRelationship(ctx context.Context, id string, opts ...GraphStoreOption) error

    // Search operations
    Search(ctx context.Context, query string, limit int, opts ...GraphSearchOption) ([]GraphSearchResult, error)
    LocalSearch(ctx context.Context, query string, entityID string, depth int, opts ...GraphSearchOption) ([]GraphSearchResult, error)
    GlobalSearch(ctx context.Context, query string, communityLevel int, opts ...GraphSearchOption) ([]GraphSearchResult, error)

    // Graph traversal
    TraverseFrom(ctx context.Context, entityID string, depth int, opts ...GraphStoreOption) (*GraphContext, error)
    ShortestPath(ctx context.Context, sourceID, targetID string, opts ...GraphStoreOption) (*GraphPath, error)

    // Extraction
    ExtractFromText(ctx context.Context, text string, llm LLM, opts ...ExtractionOption) (*ExtractionResult, error)

    // Schema
    DiscoverSchema(ctx context.Context, opts ...GraphStoreOption) (*GraphSchema, error)
    DeleteSchema(ctx context.Context) error

    // Multi-tenancy
    SetTenant(tenantID string)
    GetTenant() string

    // Lifecycle
    Close() error
}
```

### 2. Core Types

#### Entity
Represents a node in the knowledge graph:

```go
type Entity struct {
    ID          string                 // Unique identifier (required)
    Name        string                 // Human-readable name (required)
    Type        string                 // Entity type: "Person", "Organization", "Project", etc.
    Description string                 // Detailed description (used for embeddings)
    Embedding   []float32              // Vector embedding (auto-generated if embedder configured)
    Properties  map[string]interface{} // Additional metadata
    OrgID       string                 // Tenant identifier for multi-tenancy
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

#### Relationship
Represents a directed edge between entities:

```go
type Relationship struct {
    ID          string                 // Unique identifier (required)
    SourceID    string                 // Source entity ID (required)
    TargetID    string                 // Target entity ID (required)
    Type        string                 // Relationship type: "WORKS_AT", "MANAGES", etc.
    Description string                 // Relationship description
    Strength    float32                // 0.0 to 1.0 (default: 1.0)
    Properties  map[string]interface{} // Additional metadata
    OrgID       string                 // Tenant identifier
    CreatedAt   time.Time
}
```

#### GraphSearchResult
Results from knowledge graph queries:

```go
type GraphSearchResult struct {
    Entity      Entity         // The matched entity
    Score       float32        // Relevance score (0.0 to 1.0)
    Context     []Entity       // Related entities from traversal
    Path        []Relationship // Relationship path (for local search)
    CommunityID string         // Community identifier (for global search)
}
```

#### GraphContext
Context from graph traversal:

```go
type GraphContext struct {
    CentralEntity Entity         // Starting entity
    Entities      []Entity       // All discovered entities
    Relationships []Relationship // All relationships in traversal
    Depth         int            // Actual depth reached
}
```

### 3. Search Modes

| Mode | Description | Best For |
|------|-------------|----------|
| **Vector** | Semantic similarity using embeddings | Finding conceptually similar entities |
| **Keyword** | BM25 text matching | Exact term matches |
| **Hybrid** | Combines vector + keyword with RRF fusion | General-purpose (recommended default) |
| **Local** | Entity-focused with graph traversal | Questions about specific entities and their relationships |
| **Global** | Community-based aggregation | Summary queries across entity types |

### 4. Graph Traversal

The Weaviate implementation uses BFS (Breadth-First Search) for graph traversal:

```go
func (s *Store) TraverseFrom(ctx context.Context, entityID string, depth int, ...) (*GraphContext, error) {
    // 1. Start with the central entity
    // 2. For each depth level:
    //    a. Get all relationships from current entities
    //    b. Collect target entity IDs
    //    c. Fetch those entities
    // 3. Return complete subgraph
}
```

**Performance Note**: With Weaviate, each hop requires additional queries. Keep depth <= 2 for interactive use cases.

## Weaviate Schema

The Weaviate provider creates two collections:

### Entity Collection (`{ClassPrefix}Entity`)

| Property | Type | Description |
|----------|------|-------------|
| entityId | text | External unique ID |
| name | text | Entity name (tokenized for search) |
| entityType | text | Entity type for filtering |
| description | text | Description (vectorized) |
| properties | text | JSON-serialized metadata |
| orgId | text | Tenant identifier |
| createdAt | date | Creation timestamp |
| updatedAt | date | Last update timestamp |

### Relationship Collection (`{ClassPrefix}Relationship`)

| Property | Type | Description |
|----------|------|-------------|
| relationshipId | text | External unique ID |
| sourceId | text | Source entity ID |
| targetId | text | Target entity ID |
| relationshipType | text | Relationship type |
| description | text | Relationship description |
| strength | number | 0.0 to 1.0 |
| properties | text | JSON-serialized metadata |
| orgId | text | Tenant identifier |
| createdAt | date | Creation timestamp |

## Agent Integration

### Automatic Tool Registration

When `WithGraphRAG(store)` is used, five tools are automatically registered:

| Tool | Description |
|------|-------------|
| `graphrag_search` | Search entities with local/global/hybrid modes |
| `graphrag_add_entity` | Add new entities to the knowledge graph |
| `graphrag_add_relationship` | Create relationships between entities |
| `graphrag_get_context` | Get entity context via graph traversal |
| `graphrag_extract` | Extract entities/relationships from text using LLM |

### Implementation

```go
// pkg/agent/graphrag_option.go
func WithGraphRAG(store interfaces.GraphRAGStore) Option {
    return func(a *Agent) {
        a.graphRAGStore = store
        tools := createGraphRAGTools(store, a.llm)
        a.tools = deduplicateTools(append(a.tools, tools...))
    }
}
```

## Multi-Tenancy

GraphRAG supports organization-based multi-tenancy using metadata filtering:

1. **Context-based** (preferred): Use `multitenancy.WithOrgID(ctx, "org-123")`
2. **Store-level**: Use `store.SetTenant("org-123")`
3. **Per-operation**: Use `graphrag.WithTenant("org-123")` option

The tenant is stored in the `orgId` field and used to filter all queries.

## Search Flow

```
┌──────────────────┐
│   User Query     │
│ "platform team"  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  graphrag_search │
│     tool         │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Determine Mode  │
│ (hybrid default) │
└────────┬─────────┘
         │
    ┌────┴────┐
    ▼         ▼
┌───────┐ ┌───────┐
│Vector │ │Keyword│
│Search │ │Search │
│(embed)│ │(BM25) │
└───┬───┘ └───┬───┘
    │         │
    └────┬────┘
         │
         ▼
┌──────────────────┐
│  RRF Fusion      │
│ (alpha=0.5)      │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Filter by Tenant │
│ & Entity Types   │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│  Return Results  │
│ with Scores      │
└──────────────────┘
```

## File Structure

```
pkg/
├── interfaces/
│   └── graphrag.go              # Core interface definitions
├── graphrag/
│   ├── types.go                 # Type aliases (Entity, Relationship, etc.)
│   ├── options.go               # Functional options for search/store
│   ├── errors.go                # Custom error types
│   └── weaviate/
│       ├── store.go             # Main store implementation
│       ├── store_test.go        # Unit tests
│       ├── schema.go            # Weaviate collection management
│       ├── entity.go            # Entity CRUD
│       ├── relationship.go      # Relationship CRUD
│       ├── search.go            # Search implementations
│       ├── traversal.go         # Graph traversal (BFS)
│       └── extraction.go        # LLM-based extraction
├── tools/
│   └── graphrag/
│       ├── search.go            # graphrag_search tool
│       ├── add_entity.go        # graphrag_add_entity tool
│       ├── add_relationship.go  # graphrag_add_relationship tool
│       ├── get_context.go       # graphrag_get_context tool
│       └── extract.go           # graphrag_extract tool
└── agent/
    └── graphrag_option.go       # WithGraphRAG option
```

## Dependencies

- `github.com/weaviate/weaviate-go-client/v5` - Weaviate Go client
- `github.com/google/uuid` - UUID generation for entity IDs
- `pkg/embedding` - Embedding generation (optional but recommended)
- `pkg/logging` - Structured logging
- `pkg/multitenancy` - Context-based tenant management

## Performance Considerations

| Operation | Complexity | Notes |
|-----------|------------|-------|
| Store Entity | O(1) | Batch operations supported |
| Get Entity | O(1) | Direct ID lookup |
| Vector Search | O(log n) | HNSW index |
| Keyword Search | O(log n) | BM25 index |
| Graph Traversal | O(d * k) | d=depth, k=avg relationships per entity |
| Shortest Path | O(V + E) | BFS-based |

**Recommendations**:
- Keep traversal depth <= 2 for interactive queries
- Use hybrid search as default for best results
- Configure embedder for semantic search capabilities
- Use batch operations for bulk entity/relationship storage
