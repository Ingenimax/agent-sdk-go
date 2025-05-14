package supabase_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/Ingenimax/agent-sdk-go/pkg/datastore/supabase"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func setupTestClient(t *testing.T) *supabase.Client {
	// Get Supabase configuration from environment
	url := os.Getenv("SUPABASE_URL")
	if url == "" {
		t.Skip("SUPABASE_URL environment variable not set")
	}

	apiKey := os.Getenv("SUPABASE_API_KEY")
	if apiKey == "" {
		t.Skip("SUPABASE_API_KEY environment variable not set")
	}

	dbURL := os.Getenv("SUPABASE_DB_URL")
	if dbURL == "" {
		t.Skip("SUPABASE_DB_URL environment variable not set")
	}

	// Connect to database
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Create client
	client, err := supabase.New(url, apiKey, supabase.WithDB(db))
	if err != nil {
		t.Fatalf("Failed to create Supabase client: %v", err)
	}

	return client
}

func TestInsertAndGet(t *testing.T) {
	client := setupTestClient(t)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	}()

	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

	// Insert document
	collection := client.Collection("test_collection")
	data := map[string]interface{}{
		"name":  "Test Document",
		"value": 42,
	}

	id, err := collection.Insert(ctx, data)
	if err != nil {
		t.Fatalf("Failed to insert document: %v", err)
	}

	// Get document
	doc, err := collection.Get(ctx, id)
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if doc["name"] != "Test Document" {
		t.Errorf("Expected name 'Test Document', got '%v'", doc["name"])
	}
	if doc["value"] != float64(42) {
		t.Errorf("Expected value 42, got %v", doc["value"])
	}

	// Clean up
	err = collection.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	client := setupTestClient(t)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	}()

	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

	// Insert document
	collection := client.Collection("test_collection")
	data := map[string]interface{}{
		"name":  "Test Document",
		"value": 42,
	}

	id, err := collection.Insert(ctx, data)
	if err != nil {
		t.Fatalf("Failed to insert document: %v", err)
	}

	// Update document
	err = collection.Update(ctx, id, map[string]interface{}{
		"value": 43,
	})
	if err != nil {
		t.Fatalf("Failed to update document: %v", err)
	}

	// Get document
	doc, err := collection.Get(ctx, id)
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if doc["value"] != float64(43) {
		t.Errorf("Expected value 43, got %v", doc["value"])
	}

	// Clean up
	err = collection.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}
}

func TestQuery(t *testing.T) {
	client := setupTestClient(t)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	}()

	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

	// Insert documents
	collection := client.Collection("test_collection")
	data1 := map[string]interface{}{
		"name":     "Document 1",
		"category": "test",
		"value":    10,
	}
	data2 := map[string]interface{}{
		"name":     "Document 2",
		"category": "test",
		"value":    20,
	}
	data3 := map[string]interface{}{
		"name":     "Document 3",
		"category": "other",
		"value":    30,
	}

	id1, err := collection.Insert(ctx, data1)
	if err != nil {
		t.Fatalf("Failed to insert document: %v", err)
	}
	id2, err := collection.Insert(ctx, data2)
	if err != nil {
		t.Fatalf("Failed to insert document: %v", err)
	}
	id3, err := collection.Insert(ctx, data3)
	if err != nil {
		t.Fatalf("Failed to insert document: %v", err)
	}

	// Query documents
	results, err := collection.Query(ctx, map[string]interface{}{
		"category": "test",
	}, interfaces.QueryWithOrderBy("value", "asc"))
	if err != nil {
		t.Fatalf("Failed to query documents: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
	if results[0]["name"] != "Document 1" {
		t.Errorf("Expected first result to be 'Document 1', got '%v'", results[0]["name"])
	}
	if results[1]["name"] != "Document 2" {
		t.Errorf("Expected second result to be 'Document 2', got '%v'", results[1]["name"])
	}

	// Clean up
	err = collection.Delete(ctx, id1)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}
	err = collection.Delete(ctx, id2)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}
	err = collection.Delete(ctx, id3)
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}
}

func TestTransaction(t *testing.T) {
	client := setupTestClient(t)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	}()

	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

	// Execute transaction
	err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
		collection := tx.Collection("test_collection")

		// Insert document
		id, err := collection.Insert(ctx, map[string]interface{}{
			"name":  "Transaction Test",
			"value": 100,
		})
		if err != nil {
			return err
		}

		// Update document
		err = collection.Update(ctx, id, map[string]interface{}{
			"value": 101,
		})
		if err != nil {
			return err
		}

		// Get document
		doc, err := collection.Get(ctx, id)
		if err != nil {
			return err
		}

		if doc["value"] != float64(101) {
			return fmt.Errorf("expected value 101, got %v", doc["value"])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}
}
