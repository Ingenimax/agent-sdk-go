package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/Ingenimax/agent-sdk-go/pkg/datastore/supabase"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

func main() {
	// Get Supabase configuration from environment
	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		log.Fatal("SUPABASE_URL environment variable not set")
	}

	supabaseAPIKey := os.Getenv("SUPABASE_API_KEY")
	if supabaseAPIKey == "" {
		log.Fatal("SUPABASE_API_KEY environment variable not set")
	}

	supabaseDBURL := os.Getenv("SUPABASE_DB_URL")
	if supabaseDBURL == "" {
		log.Fatal("SUPABASE_DB_URL environment variable not set (needed for transactions)")
	}

	// Connect to database for transaction support
	db, err := sql.Open("postgres", supabaseDBURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Create Supabase client
	client, err := supabase.New(supabaseURL, supabaseAPIKey, supabase.WithDB(db))
	if err != nil {
		log.Fatalf("Failed to create Supabase client: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("Failed to close client: %v", err)
		}
	}()

	// Create a context with organization ID for multi-tenancy
	ctx := multitenancy.WithOrgID(context.Background(), "demo-org-123")

	// Get a collection reference
	collection := client.Collection("users")

	fmt.Println("Supabase DataStore Example")
	fmt.Println("==========================")
	fmt.Println()

	// Example 1: Insert a document
	fmt.Println("1. Inserting a document...")
	userData := map[string]interface{}{
		"name":   "Alice Johnson",
		"email":  "alice@example.com",
		"age":    28,
		"status": "active",
	}

	userID, err := collection.Insert(ctx, userData)
	if err != nil {
		log.Fatalf("Failed to insert document: %v", err)
	}
	fmt.Printf("   Inserted user with ID: %s\n", userID)

	// Example 2: Get a document by ID
	fmt.Println("2. Retrieving the document...")
	doc, err := collection.Get(ctx, userID)
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}
	fmt.Printf("   Retrieved user: %+v\n", doc)

	// Example 3: Update a document
	fmt.Println("3. Updating the document...")
	updateData := map[string]interface{}{
		"age":    29,
		"status": "verified",
	}
	err = collection.Update(ctx, userID, updateData)
	if err != nil {
		log.Fatalf("Failed to update document: %v", err)
	}
	fmt.Println("   Document updated successfully")

	// Example 4: Get updated document
	fmt.Println("4. Retrieving updated document...")
	updatedDoc, err := collection.Get(ctx, userID)
	if err != nil {
		log.Fatalf("Failed to get updated document: %v", err)
	}
	fmt.Printf("   Updated user: %+v\n\n", updatedDoc)

	// Example 5: Insert multiple documents
	fmt.Println("5. Inserting multiple documents...")
	users := []map[string]interface{}{
		{
			"name":   "Bob Smith",
			"email":  "bob@example.com",
			"age":    35,
			"status": "active",
		},
		{
			"name":   "Carol White",
			"email":  "carol@example.com",
			"age":    42,
			"status": "active",
		},
		{
			"name":   "David Brown",
			"email":  "david@example.com",
			"age":    31,
			"status": "inactive",
		},
	}

	var userIDs []string
	for _, user := range users {
		id, err := collection.Insert(ctx, user)
		if err != nil {
			log.Fatalf("Failed to insert user: %v", err)
		}
		userIDs = append(userIDs, id)
		fmt.Printf("   Inserted user: %s with ID: %s\n", user["name"], id)
	}
	fmt.Println()

	// Example 6: Query documents
	fmt.Println("6. Querying active users...")
	activeDocs, err := collection.Query(ctx, map[string]interface{}{
		"status": "active",
	})
	if err != nil {
		log.Fatalf("Failed to query documents: %v", err)
	}
	fmt.Printf("   Found %d active users\n", len(activeDocs))
	for _, doc := range activeDocs {
		fmt.Printf("   - %s (%s)\n", doc["name"], doc["email"])
	}
	fmt.Println()

	// Example 7: Query with limit and ordering
	fmt.Println("7. Querying with limit and ordering...")
	limitedDocs, err := collection.Query(ctx,
		map[string]interface{}{
			"status": "active",
		},
		interfaces.QueryWithOrderBy("name", "asc"),
		interfaces.QueryWithLimit(2),
	)
	if err != nil {
		log.Fatalf("Failed to query with limit: %v", err)
	}
	fmt.Printf("   Retrieved top 2 active users (ordered by name):\n")
	for _, doc := range limitedDocs {
		fmt.Printf("   - %s\n", doc["name"])
	}
	fmt.Println()

	// Example 8: Transaction - Insert multiple related documents
	fmt.Println("8. Using transactions...")
	var txUserID string
	err = client.Transaction(ctx, func(tx interfaces.Transaction) error {
		txCollection := tx.Collection("users")

		// Insert user in transaction
		id, err := txCollection.Insert(ctx, map[string]interface{}{
			"name":   "Eve Transaction",
			"email":  "eve@example.com",
			"age":    27,
			"status": "active",
		})
		if err != nil {
			return err
		}
		txUserID = id

		// Update user in same transaction
		err = txCollection.Update(ctx, id, map[string]interface{}{
			"status": "verified",
		})
		if err != nil {
			return err
		}

		fmt.Println("   Transaction completed successfully")
		return nil
	})

	if err != nil {
		log.Fatalf("Transaction failed: %v", err)
	}
	userIDs = append(userIDs, txUserID)
	fmt.Println()

	// Example 9: Clean up - Delete all created documents
	fmt.Println("9. Cleaning up - deleting all created documents...")
	allUserIDs := append([]string{userID}, userIDs...)
	for _, id := range allUserIDs {
		err = collection.Delete(ctx, id)
		if err != nil {
			log.Printf("Warning: Failed to delete document %s: %v", id, err)
		} else {
			fmt.Printf("   Deleted user with ID: %s\n", id)
		}
	}
	fmt.Println()

	fmt.Println("Example completed successfully!")
}
