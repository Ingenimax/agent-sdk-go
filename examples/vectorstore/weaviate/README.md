### Weaviate Vector Store Example
This example demonstrates how to use Weaviate as a vector store with the Agent SDK. It shows basic operations like storing, searching, and deleting documents.
## Prerequisites
Before running the example, you'll need:
1. An OpenAI API key (for text embeddings)
2. Weaviate running locally or in the cloud

## Setup

Set environment variables:
```bash
# Required for Weaviate with text2vec-openai
export OPENAI_API_KEY=your_openai_api_key

# Weaviate connection details
export WEAVIATE_HOST=localhost:8080
export WEAVIATE_API_KEY=your_weaviate_api_key  # If authentication is enabled
```

2. Start Weaviate:

```bash
docker run -d --name weaviate \
  -p 8080:8080 \
  -e AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED=true \
  -e DEFAULT_VECTORIZER_MODULE=text2vec-openai \
  -e ENABLE_MODULES=text2vec-openai \
  -e OPENAI_APIKEY=$OPENAI_API_KEY \
  semitechnologies/weaviate:1.19.6
```

## Running the Example

Run the compiled binary:

```bash
go build -o weaviate_example cmd/examples/vectorstore/weaviate/main.go
./weaviate_example
```

## Example Code

The example demonstrates:

1. Connecting to Weaviate
2. Storing documents with metadata
3. Searching for similar documents
4. Filtering search results
5. Deleting documents

```go:cmd/examples/vectorstore/weaviate/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	"github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
)

func main() {
	// Get Weaviate configuration from environment
	host := os.Getenv("WEAVIATE_HOST")
	if host == "" {
		host = "localhost:8080" // Default Weaviate host
	}

	apiKey := os.Getenv("WEAVIATE_API_KEY")
	// API key is optional for local development

	// Create vector store
	store := weaviate.New(
		&interfaces.VectorStoreConfig{
			Host:   host,
			APIKey: apiKey,
		},
		weaviate.WithClassPrefix("Example"),
	)

	// Create context with organization ID
	ctx := multitenancy.WithOrgID(context.Background(), "example-org")

	// Store some documents
	docs := []interfaces.Document{
		{
			ID:      "doc1",
			Content: "The quick brown fox jumps over the lazy dog",
			Metadata: map[string]interface{}{
				"source": "example",
				"type":   "pangram",
			},
		},
		{
			ID:      "doc2",
			Content: "To be or not to be, that is the question",
			Metadata: map[string]interface{}{
				"source": "example",
				"type":   "quote",
			},
		},
	}

	fmt.Println("Storing documents...")
	if err := store.Store(ctx, docs); err != nil {
		log.Fatalf("Failed to store documents: %v", err)
	}

	// Search for similar documents
	fmt.Println("\nSearching for 'fox jumps'...")
	results, err := store.Search(ctx, "fox jumps", 5)
	if err != nil {
		log.Fatalf("Failed to search: %v", err)
	}

	fmt.Println("Search results:")
	for _, result := range results {
		fmt.Printf("- %s (score: %.2f)\n", result.Document.Content, result.Score)
	}

	// Search with filters
	fmt.Println("\nSearching with filters for type=pangram...")
	filteredResults, err := store.Search(ctx, "fox jumps", 5,
		interfaces.WithFilters(map[string]interface{}{
			"type": "pangram",
		}),
	)
	if err != nil {
		log.Fatalf("Failed to search with filters: %v", err)
	}

	fmt.Println("Filtered search results:")
	for _, result := range filteredResults {
		fmt.Printf("- %s (score: %.2f)\n", result.Document.Content, result.Score)
	}

	// Get documents by ID
	fmt.Println("\nGetting document by ID...")
	retrieved, err := store.Get(ctx, []string{"doc1"})
	if err != nil {
		log.Fatalf("Failed to get document: %v", err)
	}

	fmt.Println("Retrieved document:")
	for _, doc := range retrieved {
		fmt.Printf("- ID: %s, Content: %s\n", doc.ID, doc.Content)
	}

	// Clean up
	fmt.Println("\nDeleting documents...")
	err = store.Delete(ctx, []string{"doc1", "doc2"})
	if err != nil {
		log.Fatalf("Failed to delete documents: %v", err)
	}
	fmt.Println("Documents deleted successfully")
}
```

## Expected Output

```
Storing documents...

Searching for 'fox jumps'...
Search results:
- The quick brown fox jumps over the lazy dog (score: 0.95)
- To be or not to be, that is the question (score: 0.65)

Searching with filters for type=pangram...
Filtered search results:
- The quick brown fox jumps over the lazy dog (score: 0.95)

Getting document by ID...
Retrieved document:
- ID: doc1, Content: The quick brown fox jumps over the lazy dog

Deleting documents...
Documents deleted successfully
```

## Troubleshooting

If you encounter issues:

1. **Weaviate Connection**:
   - Verify Weaviate is running: `curl http://localhost:8080/v1/.well-known/ready`
   - Check Docker logs: `docker logs weaviate`

2. **Dependency Issues**:
   - If you see errors related to missing functions like `byteops.Float32ToByteVector`, try using an older version of the Weaviate client (v4.15.0)

3. **OpenAI API Key**:
   - Ensure your OpenAI API key is valid and has been provided to Weaviate

4. **Class Creation**:
   - If you see errors about classes not existing, check that the example has permission to create classes in Weaviate

5. **Verbose Logging**:
   - Set `WEAVIATE_VERBOSE=true` for more detailed logs