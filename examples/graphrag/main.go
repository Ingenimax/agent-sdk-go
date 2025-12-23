// Example program to test GraphRAG functionality
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/graphrag/weaviate"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func main() {
	ctx := context.Background()

	// Create GraphRAG store
	store, err := weaviate.New(&weaviate.Config{
		Host:        "localhost:8080",
		Scheme:      "http",
		ClassPrefix: "Example",
	})
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	fmt.Println("=== GraphRAG Test ===")

	// 1. Store entities
	fmt.Println("\n1. Storing entities...")
	entities := []interfaces.Entity{
		{
			ID:          "person-1",
			Name:        "John Smith",
			Type:        "Person",
			Description: "Software engineer specializing in AI and machine learning",
			Properties:  map[string]interface{}{"role": "Engineer", "department": "R&D"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "company-1",
			Name:        "TechCorp",
			Type:        "Organization",
			Description: "Technology company focusing on artificial intelligence solutions",
			Properties:  map[string]interface{}{"industry": "Technology", "size": "Enterprise"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			ID:          "project-1",
			Name:        "AI Assistant",
			Type:        "Project",
			Description: "Building an intelligent assistant using large language models",
			Properties:  map[string]interface{}{"status": "Active", "priority": "High"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	}

	if err := store.StoreEntities(ctx, entities); err != nil {
		log.Fatalf("Failed to store entities: %v", err)
	}
	fmt.Printf("   Stored %d entities\n", len(entities))

	// 2. Store relationships
	fmt.Println("\n2. Storing relationships...")
	relationships := []interfaces.Relationship{
		{
			ID:          "rel-1",
			SourceID:    "person-1",
			TargetID:    "company-1",
			Type:        "WORKS_AT",
			Description: "John works at TechCorp",
			Strength:    1.0,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "rel-2",
			SourceID:    "person-1",
			TargetID:    "project-1",
			Type:        "WORKS_ON",
			Description: "John is the lead developer on AI Assistant",
			Strength:    0.9,
			CreatedAt:   time.Now(),
		},
		{
			ID:          "rel-3",
			SourceID:    "company-1",
			TargetID:    "project-1",
			Type:        "OWNS",
			Description: "TechCorp owns the AI Assistant project",
			Strength:    1.0,
			CreatedAt:   time.Now(),
		},
	}

	if err := store.StoreRelationships(ctx, relationships); err != nil {
		log.Fatalf("Failed to store relationships: %v", err)
	}
	fmt.Printf("   Stored %d relationships\n", len(relationships))

	// Give Weaviate a moment to index
	time.Sleep(500 * time.Millisecond)

	// 3. Get entity
	fmt.Println("\n3. Getting entity by ID...")
	entity, err := store.GetEntity(ctx, "person-1")
	if err != nil {
		log.Fatalf("Failed to get entity: %v", err)
	}
	fmt.Printf("   Found: %s (%s) - %s\n", entity.Name, entity.Type, entity.Description)

	// 4. Get relationships
	fmt.Println("\n4. Getting relationships for John...")
	rels, err := store.GetRelationships(ctx, "person-1", interfaces.DirectionBoth)
	if err != nil {
		log.Fatalf("Failed to get relationships: %v", err)
	}
	for _, rel := range rels {
		fmt.Printf("   %s -[%s]-> %s\n", rel.SourceID, rel.Type, rel.TargetID)
	}

	// 5. Search (keyword mode since no embedder configured)
	fmt.Println("\n5. Searching for 'artificial intelligence'...")
	results, err := store.Search(ctx, "artificial intelligence", 5, interfaces.WithSearchMode(interfaces.SearchModeKeyword))
	if err != nil {
		log.Fatalf("Failed to search: %v", err)
	}
	fmt.Printf("   Found %d results:\n", len(results))
	for _, r := range results {
		fmt.Printf("   - %s (%s): %.2f\n", r.Entity.Name, r.Entity.Type, r.Score)
	}

	// 6. Traverse graph
	fmt.Println("\n6. Traversing graph from John (depth 2)...")
	graphCtx, err := store.TraverseFrom(ctx, "person-1", 2)
	if err != nil {
		log.Fatalf("Failed to traverse: %v", err)
	}
	fmt.Printf("   Central entity: %s\n", graphCtx.CentralEntity.Name)
	fmt.Printf("   Connected entities: %d\n", len(graphCtx.Entities))
	for _, e := range graphCtx.Entities {
		fmt.Printf("   - %s (%s)\n", e.Name, e.Type)
	}
	fmt.Printf("   Relationships: %d\n", len(graphCtx.Relationships))
	for _, r := range graphCtx.Relationships {
		fmt.Printf("   - %s -[%s]-> %s\n", r.SourceID, r.Type, r.TargetID)
	}

	// 7. Discover schema
	fmt.Println("\n7. Discovering schema...")
	schema, err := store.DiscoverSchema(ctx)
	if err != nil {
		log.Fatalf("Failed to discover schema: %v", err)
	}
	fmt.Printf("   Entity types: %v\n", getEntityTypeNames(schema))
	fmt.Printf("   Relationship types: %v\n", getRelationshipTypeNames(schema))

	// 8. Clean up
	fmt.Println("\n8. Cleaning up...")
	if err := store.DeleteSchema(ctx); err != nil {
		log.Printf("Warning: Failed to delete schema: %v", err)
	}
	fmt.Println("   Done!")

	fmt.Println("\n=== All tests passed! ===")
}

func getEntityTypeNames(schema *interfaces.GraphSchema) []string {
	names := make([]string, len(schema.EntityTypes))
	for i, et := range schema.EntityTypes {
		names[i] = et.Name
	}
	return names
}

func getRelationshipTypeNames(schema *interfaces.GraphSchema) []string {
	names := make([]string, len(schema.RelationshipTypes))
	for i, rt := range schema.RelationshipTypes {
		names[i] = rt.Name
	}
	return names
}
