package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/Ingenimax/agent-sdk-go/pkg/datastore/postgres"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func setupTestClient(t *testing.T) *postgres.Client {
	// Get PostgreSQL configuration from environment
	dbURL := os.Getenv("POSTGRES_URL")
	if dbURL == "" {
		t.Skip("POSTGRES_URL environment variable not set")
	}

	// Create client
	client, err := postgres.New(dbURL)
	if err != nil {
		t.Fatalf("Failed to create PostgreSQL client: %v", err)
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
	if doc["value"] != int64(42) {
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
		"name":  "Original Name",
		"value": 100,
	}

	id, err := collection.Insert(ctx, data)
	if err != nil {
		t.Fatalf("Failed to insert document: %v", err)
	}

	// Update document
	updateData := map[string]interface{}{
		"name":  "Updated Name",
		"value": 200,
	}

	err = collection.Update(ctx, id, updateData)
	if err != nil {
		t.Fatalf("Failed to update document: %v", err)
	}

	// Get updated document
	doc, err := collection.Get(ctx, id)
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if doc["name"] != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%v'", doc["name"])
	}
	if doc["value"] != int64(200) {
		t.Errorf("Expected value 200, got %v", doc["value"])
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
	collection := client.Collection("test_collection")

	// Insert multiple documents
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		data := map[string]interface{}{
			"name":     "Test Document",
			"category": "test",
			"value":    (i + 1) * 10,
		}

		id, err := collection.Insert(ctx, data)
		if err != nil {
			t.Fatalf("Failed to insert document: %v", err)
		}
		ids[i] = id
	}

	// Query documents
	docs, err := collection.Query(ctx, map[string]interface{}{
		"category": "test",
	})
	if err != nil {
		t.Fatalf("Failed to query documents: %v", err)
	}

	if len(docs) < 3 {
		t.Errorf("Expected at least 3 documents, got %d", len(docs))
	}

	// Clean up
	for _, id := range ids {
		err = collection.Delete(ctx, id)
		if err != nil {
			t.Errorf("Failed to delete document: %v", err)
		}
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

	var id1, id2 string

	// Execute transaction
	err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
		collection := tx.Collection("test_collection")

		// Insert first document
		var err error
		id1, err = collection.Insert(ctx, map[string]interface{}{
			"name":  "Transaction Doc 1",
			"value": 100,
		})
		if err != nil {
			return err
		}

		// Insert second document
		id2, err = collection.Insert(ctx, map[string]interface{}{
			"name":  "Transaction Doc 2",
			"value": 200,
		})
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Verify documents were inserted
	collection := client.Collection("test_collection")

	doc1, err := collection.Get(ctx, id1)
	if err != nil {
		t.Errorf("Failed to get first document: %v", err)
	} else if doc1["name"] != "Transaction Doc 1" {
		t.Errorf("Expected name 'Transaction Doc 1', got '%v'", doc1["name"])
	}

	doc2, err := collection.Get(ctx, id2)
	if err != nil {
		t.Errorf("Failed to get second document: %v", err)
	} else if doc2["name"] != "Transaction Doc 2" {
		t.Errorf("Expected name 'Transaction Doc 2', got '%v'", doc2["name"])
	}

	// Clean up
	_ = collection.Delete(ctx, id1)
	_ = collection.Delete(ctx, id2)
}

func TestTransactionRollback(t *testing.T) {
	client := setupTestClient(t)
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	}()

	ctx := multitenancy.WithOrgID(context.Background(), "test-org")

	var id1 string

	// Execute transaction that should fail
	err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
		collection := tx.Collection("test_collection")

		// Insert first document
		var err error
		id1, err = collection.Insert(ctx, map[string]interface{}{
			"name":  "Rollback Doc",
			"value": 100,
		})
		if err != nil {
			return err
		}

		// Return error to trigger rollback
		return sql.ErrConnDone
	})

	if err == nil {
		t.Fatal("Expected transaction to fail")
	}

	// Verify document was not inserted
	collection := client.Collection("test_collection")
	_, err = collection.Get(ctx, id1)
	if err == nil {
		t.Error("Document should not exist after rollback")
		// Clean up if it exists
		_ = collection.Delete(ctx, id1)
	}
}
