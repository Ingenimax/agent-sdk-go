package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
	weaviateSdk "github.com/Ingenimax/agent-sdk-go/pkg/vectorstore/weaviate"
	"github.com/google/uuid"
	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
)

func main() {
	// Create a logger
	baseLogger := logging.New()

	// Apply debug level to logger
	debugOption := logging.WithLevel("debug")
	debugOption(baseLogger)
	logger := baseLogger

	ctx := multitenancy.WithOrgID(context.Background(), "exampleorg")

	// Load configuration
	cfg := config.Get()

	// For Weaviate Cloud, ensure we're using HTTPS
	if strings.Contains(cfg.VectorStore.Weaviate.Host, ".weaviate.cloud") && cfg.VectorStore.Weaviate.Scheme != "https" {
		logger.Warn(ctx, "Weaviate Cloud detected but scheme is not HTTPS, updating to HTTPS", map[string]interface{}{
			"original_scheme": cfg.VectorStore.Weaviate.Scheme,
			"host":            cfg.VectorStore.Weaviate.Host,
		})
		cfg.VectorStore.Weaviate.Scheme = "https"
	}

	// Check if API key is provided for Weaviate Cloud
	if strings.Contains(cfg.VectorStore.Weaviate.Host, ".weaviate.cloud") && cfg.VectorStore.Weaviate.APIKey == "" {
		logger.Error(ctx, "Weaviate Cloud detected but no API key provided", nil)
		logger.Info(ctx, "Please set your Weaviate API key in the configuration", nil)
		return
	}

	// Check if OpenAI API key is provided for Weaviate Cloud
	if strings.Contains(cfg.VectorStore.Weaviate.Host, ".weaviate.cloud") && cfg.LLM.OpenAI.APIKey == "" {
		logger.Error(ctx, "Weaviate Cloud detected but no OpenAI API key provided", nil)
		logger.Info(ctx, "Please set your OpenAI API key in the configuration for Weaviate Cloud", nil)
		return
	}

	// Try to connect to Weaviate
	logger.Info(ctx, "Validating Weaviate connection...", nil)
	weaviateURL := fmt.Sprintf("%s://%s", cfg.VectorStore.Weaviate.Scheme, cfg.VectorStore.Weaviate.Host)

	logger.Debug(ctx, "Attempting to connect to Weaviate", map[string]interface{}{
		"url":                weaviateURL,
		"has_api_key":        cfg.VectorStore.Weaviate.APIKey != "",
		"has_openai_api_key": cfg.LLM.OpenAI.APIKey != "",
	})

	err := validateWeaviateConnection(weaviateURL)
	if err != nil {
		logger.Error(ctx, "Failed to connect to Weaviate", map[string]interface{}{
			"error": err.Error(),
			"url":   weaviateURL,
		})

		// Provide more specific guidance for Weaviate Cloud
		if strings.Contains(cfg.VectorStore.Weaviate.Host, ".weaviate.cloud") {
			logger.Info(ctx, "For Weaviate Cloud, ensure you have:", nil)
			logger.Info(ctx, "1. Correct API key in your configuration", nil)
			logger.Info(ctx, "2. HTTPS scheme (not HTTP)", nil)
			logger.Info(ctx, "3. OpenAI API key if using OpenAI embeddings", nil)
		} else {
			logger.Info(ctx, "Please check your Weaviate configuration and ensure the service is running", nil)
		}
		return
	}

	logger.Info(ctx, "Weaviate connection successful", map[string]interface{}{"url": weaviateURL})

	// Initialize the OpenAIEmbedder with custom configuration
	embeddingConfig := embedding.DefaultEmbeddingConfig(cfg.LLM.OpenAI.EmbeddingModel)
	embeddingConfig.Dimensions = 1536 // Specify dimensions for more precise embeddings
	embeddingConfig.SimilarityMetric = "cosine"
	embeddingConfig.SimilarityThreshold = 0.6 // Set a similarity threshold

	embedder := embedding.NewOpenAIEmbedderWithConfig(cfg.LLM.OpenAI.APIKey, embeddingConfig)

	// Create vector store options
	storeOptions := []weaviateSdk.Option{
		weaviateSdk.WithEmbedder(embedder),
		weaviateSdk.WithClassPrefix("Example"),
		weaviateSdk.WithLogger(logger),
	}

	// For Weaviate Cloud, log that we're using OpenAI embeddings
	if strings.Contains(cfg.VectorStore.Weaviate.Host, ".weaviate.cloud") && cfg.LLM.OpenAI.APIKey != "" {
		logger.Debug(ctx, "Using OpenAI embeddings with Weaviate Cloud", map[string]interface{}{
			"embedding_model": cfg.LLM.OpenAI.EmbeddingModel,
		})
	}

	// Create the store
	store := weaviateSdk.New(
		&interfaces.VectorStoreConfig{
			Host:   cfg.VectorStore.Weaviate.Host,
			APIKey: cfg.VectorStore.Weaviate.APIKey,
			Scheme: cfg.VectorStore.Weaviate.Scheme,
		},
		storeOptions...,
	)

	// Example documents
	documents := []interfaces.Document{
		{
			ID:      uuid.New().String(),
			Content: "Artificial intelligence (AI) is intelligence demonstrated by machines, as opposed to natural intelligence displayed by animals including humans.",
			Metadata: map[string]interface{}{
				"source": "wikipedia",
				"topic":  "AI",
				"year":   2023,
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "Machine learning is a subset of artificial intelligence that focuses on the development of algorithms that can learn from and make predictions based on data.",
			Metadata: map[string]interface{}{
				"source": "textbook",
				"topic":  "machine learning",
				"year":   2022,
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "Deep learning is a subset of machine learning that uses neural networks with many layers to analyze various factors of data.",
			Metadata: map[string]interface{}{
				"source": "research paper",
				"topic":  "deep learning",
				"year":   2021,
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "Natural language processing (NLP) is a subfield of linguistics, computer science, and artificial intelligence concerned with the interactions between computers and human language.",
			Metadata: map[string]interface{}{
				"source": "academic journal",
				"topic":  "NLP",
				"year":   2023,
			},
		},
		{
			ID:      uuid.New().String(),
			Content: "Computer vision is an interdisciplinary scientific field that deals with how computers can gain high-level understanding from digital images or videos.",
			Metadata: map[string]interface{}{
				"source": "textbook",
				"topic":  "computer vision",
				"year":   2020,
			},
		},
	}

	// Embedding a single text
	logger.Info(ctx, "Embedding a single text...", nil)
	vector, err := embedder.Embed(ctx, "What is artificial intelligence?")
	if err != nil {
		logger.Error(ctx, "Embedding failed", map[string]interface{}{"error": err.Error()})
		return
	}
	logger.Info(ctx, fmt.Sprintf("Generated embedding with %d dimensions", len(vector)), nil)

	// Storing documents
	logger.Info(ctx, "Storing documents...", nil)
	err = store.Store(ctx, documents)
	if err != nil {
		logger.Error(ctx, "Failed to store documents", map[string]interface{}{
			"error":  err.Error(),
			"host":   cfg.VectorStore.Weaviate.Host,
			"scheme": cfg.VectorStore.Weaviate.Scheme,
		})
		logger.Info(ctx, "Skipping search operations due to storage failure", nil)
		return
	}
	logger.Info(ctx, "Successfully stored documents", nil)

	// Validate that documents were properly stored
	logger.Info(ctx, "Validating document storage...", nil)
	if !validateDocumentsStored(ctx, logger, store, documents) {
		logger.Warn(ctx, "Document validation failed, search results may be affected", nil)
	} else {
		logger.Info(ctx, "Document validation successful, proceeding with search operations", nil)
	}

	// Check the schema to ensure the class is properly configured
	logger.Info(ctx, "Checking Weaviate schema...", nil)
	// TODO: Uncomment when function is implemented
	// checkWeaviateSchema(ctx, logger, store)

	// Add a delay to ensure documents are indexed
	logger.Info(ctx, "Waiting for documents to be indexed...", nil)
	time.Sleep(3 * time.Second)

	// Basic search
	logger.Info(ctx, "Performing basic search...", nil)

	// Use a simpler query for testing
	searchQuery := "artificial intelligence"

	logger.Debug(ctx, "Search query details", map[string]interface{}{
		"query": searchQuery,
		"limit": 3,
		"class": "Example_exampleorg",
	})

	// Use direct Weaviate search instead of standard search
	/* 	logger.Info(ctx, "Using direct Weaviate search approach...", nil)
	   	basicSearchResults := performDirectVectorSearch(ctx, logger, searchQuery, 3) */

	/* 	if basicSearchResults != nil && len(basicSearchResults) > 0 {
	   		logger.Info(ctx, "Direct search results:", nil)
	   		printResults(ctx, logger, basicSearchResults)

	   		// Continue with other search operations using our direct approach
	   		// Search with filters for "AI" query
	   		logger.Info(ctx, "Performing search with 'AI' query for filtering...", nil)
	   		aiResults := performDirectVectorSearch(ctx, logger, "AI", 10) // Get more results for filtering

	   		if aiResults != nil && len(aiResults) > 0 {
	   			logger.Info(ctx, "Found results for 'AI' query, applying filters...", nil)

	   			// Use the embedding package's metadata filtering capabilities
	   			// Create filter groups for different criteria
	   			wikipediaFilter := embedding.NewMetadataFilterGroup("and",
	   				embedding.NewMetadataFilter("source", "=", "wikipedia"))

	   			textbookFilter := embedding.NewMetadataFilterGroup("and",
	   				embedding.NewMetadataFilter("source", "=", "textbook"))

	   			recentFilter := embedding.NewMetadataFilterGroup("and",
	   				embedding.NewMetadataFilter("year", ">", 2021))

	   			nlpFilter := embedding.NewMetadataFilterGroup("and",
	   				embedding.NewMetadataFilter("topic", "=", "NLP"))

	   			// Apply filters using the embedding package
	   			wikipediaResults := embedding.ApplyFilters(convertToDocuments(aiResults), wikipediaFilter)
	   			textbookResults := embedding.ApplyFilters(convertToDocuments(aiResults), textbookFilter)
	   			recentResults := embedding.ApplyFilters(convertToDocuments(aiResults), recentFilter)
	   			nlpResults := embedding.ApplyFilters(convertToDocuments(aiResults), nlpFilter)

	   			// Convert back to search results for display
	   			logger.Info(ctx, "Search results with source=wikipedia filter:", nil)
	   			printResults(ctx, logger, convertToSearchResults(wikipediaResults))

	   			logger.Info(ctx, "Search results with source=textbook filter:", nil)
	   			printResults(ctx, logger, convertToSearchResults(textbookResults))

	   			logger.Info(ctx, "Search results with year > 2021 filter:", nil)
	   			printResults(ctx, logger, convertToSearchResults(recentResults))

	   			logger.Info(ctx, "Search results with topic=NLP filter:", nil)
	   			printResults(ctx, logger, convertToSearchResults(nlpResults))
	   		} else {
	   			logger.Error(ctx, "Search with 'AI' query failed", nil)
	   		}
	   	} else {
	   		// Fall back to direct search with complex filter
	   		logger.Info(ctx, "Basic search failed, trying direct search with complex filter...", nil)
	   		searchWithDirectWeaviateFilter(ctx, logger, searchQuery, 3)
	   	}
	*/
	// Calculate similarity between two texts
	logger.Info(ctx, "Calculating similarity between two texts...", nil)
	text1 := "What is artificial intelligence?"
	text2 := "AI is the simulation of human intelligence by machines"

	// Get embeddings for both texts
	vec1, err := embedder.Embed(ctx, text1)
	if err != nil {
		logger.Error(ctx, "Embedding for similarity calculation failed", map[string]interface{}{"error": err.Error()})
		return
	}

	vec2, err := embedder.Embed(ctx, text2)
	if err != nil {
		logger.Error(ctx, "Embedding for similarity calculation failed", map[string]interface{}{"error": err.Error()})
		return
	}

	// Calculate cosine similarity
	similarity, err := calculateCosineSimilarity(vec1, vec2)
	if err != nil {
		logger.Error(ctx, "Similarity calculation failed", map[string]interface{}{"error": err.Error()})
		return
	}
	logger.Info(ctx, fmt.Sprintf("Similarity score: %.4f", similarity), nil)

	// Demonstrate using the embedding package's filter capabilities with Weaviate
	logger.Info(ctx, "Demonstrating embedding package's filter capabilities with Weaviate...", nil)

	// Create a complex filter group
	complexFilter := embedding.NewMetadataFilterGroup("and",
		embedding.NewMetadataFilter("year", ">", 2020))

	// Add a nested OR group for sources
	sourceFilter := embedding.NewMetadataFilterGroup("or",
		embedding.NewMetadataFilter("source", "=", "wikipedia"),
		embedding.NewMetadataFilter("source", "=", "textbook"))

	complexFilter.AddSubGroup(sourceFilter)

	// Try searching with the complex filter
	logger.Info(ctx, "Searching with complex filter (year > 2020 AND (source = wikipedia OR source = textbook))...", nil)

	// First try with direct Weaviate filter builder approach (this works reliably)
	logger.Info(ctx, "Using direct Weaviate filter builder approach (most reliable)...", nil)
	searchWithDirectWeaviateFilter(ctx, logger, "AI", 5)

	// Then try with manual filtering using the embedding package
	logger.Info(ctx, "Using manual filtering with embedding package...", nil)

	// Use direct vector search instead of standard search
	//allResults := performDirectVectorSearch(ctx, logger, "AI", 10)

	/* 	if allResults != nil && len(allResults) > 0 {
		// Apply the complex filter manually
		filteredDocs := embedding.ApplyFilters(convertToDocuments(allResults), complexFilter)

		logger.Info(ctx, "Search results using manual filtering with embedding package:", nil)
		printResults(ctx, logger, convertToSearchResults(filteredDocs))
	} */

	// Finally try with the SDK's filter format (may not work on all Weaviate instances)
	logger.Info(ctx, "Trying SDK's filter format (may not work on all Weaviate instances)...", nil)

	// First try with direct Weaviate filter format
	weaviateFilterResults, err := searchWithWeaviateFilters(ctx, logger, "AI", 5, complexFilter)
	if err != nil {
		logger.Warn(ctx, "Search with Weaviate filters failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		logger.Info(ctx, "Search results using Weaviate filter format:", nil)
		printResults(ctx, logger, weaviateFilterResults)
	}

	// Then try with vector search and Weaviate filter format
	vectorFilterResults, err := searchWithVectorAndWeaviateFilters(ctx, logger, embedder, "AI", 5, complexFilter)
	if err != nil {
		logger.Warn(ctx, "Vector search with Weaviate filters failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		logger.Info(ctx, "Vector search results using Weaviate filter format:", nil)
		printResults(ctx, logger, vectorFilterResults)
	}

	// Batch embedding
	logger.Info(ctx, "Performing batch embedding...", nil)
	texts := []string{
		"What is machine learning?",
		"How does deep learning work?",
		"What are neural networks?",
	}
	vectors, err := embedder.EmbedBatch(ctx, texts)
	if err != nil {
		logger.Error(ctx, "Batch embedding failed", map[string]interface{}{"error": err.Error()})
		return
	}
	logger.Info(ctx, fmt.Sprintf("Generated %d embeddings in batch", len(vectors)), nil)

	// Try getting all documents as a final operation before cleanup
	logger.Info(ctx, "Trying to get all documents as a final operation...", nil)
	tryGetAllDocuments(ctx, logger, store, documents)

	// Clean up
	logger.Info(ctx, "Cleaning up...", nil)
	ids := make([]string, len(documents))
	for i, doc := range documents {
		ids[i] = doc.ID
	}
	err = store.Delete(ctx, ids)
	if err != nil {
		logger.Error(ctx, "Cleanup failed", map[string]interface{}{"error": err.Error()})
		return
	}
	logger.Info(ctx, "Successfully cleaned up documents", nil)
}

func convertToSearchResults(docs []interfaces.Document) []interfaces.SearchResult {
	results := make([]interfaces.SearchResult, len(docs))
	for i, doc := range docs {
		results[i] = interfaces.SearchResult{
			Document: doc,
			Score:    1.0 - float32(i)*0.1, // Simple scoring based on position
		}
	}
	return results
}

// printResults prints search results in a readable format
func printResults(ctx context.Context, logger logging.Logger, results []interfaces.SearchResult) {
	if len(results) == 0 {
		logger.Info(ctx, "No results found", nil)
		return
	}

	logger.Info(ctx, fmt.Sprintf("Found %d results:", len(results)), nil)
	for i, result := range results {
		logger.Info(ctx, fmt.Sprintf("%d. %s (Score: %.4f)", i+1, truncateString(result.Document.Content, 100), result.Score), nil)
		logger.Info(ctx, fmt.Sprintf("   Metadata: %v", result.Document.Metadata), nil)
	}
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// searchWithWeaviateFilters performs a search using the Weaviate filter format
func searchWithWeaviateFilters(ctx context.Context, logger logging.Logger,
	query string, limit int, filterGroup embedding.MetadataFilterGroup) ([]interfaces.SearchResult, error) {

	logger.Info(ctx, "Attempting search with Weaviate filters", map[string]interface{}{
		"query": query,
		"limit": limit,
	})

	// Create a robust client
	client, className, err := createRobustWeaviateClient(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create robust client: %w", err)
	}

	// Generate embedding for the query
	cfg := config.Get()
	embedder := embedding.NewOpenAIEmbedderWithConfig(cfg.LLM.OpenAI.APIKey, embedding.DefaultEmbeddingConfig(cfg.LLM.OpenAI.EmbeddingModel))

	queryVector, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for query: %w", err)
	}

	// Create a direct Weaviate filter instead of trying to convert from a map
	var whereFilter *filters.WhereBuilder

	// Check if we have a simple filter for a single field
	if len(filterGroup.Filters) == 1 && len(filterGroup.SubGroups) == 0 {
		// Simple case: single filter
		filter := filterGroup.Filters[0]
		whereFilter = createSimpleFilter(filter)
	} else if strings.ToLower(filterGroup.Operator) == "and" || strings.ToLower(filterGroup.Operator) == "or" {
		// Complex case: AND/OR with multiple conditions
		whereFilter = createComplexFilter(ctx, logger, filterGroup)
	}

	// If we couldn't create a filter, fall back to manual filtering
	if whereFilter == nil {
		logger.Warn(ctx, "Failed to create Weaviate filter, falling back to manual filtering", nil)
		return searchWithManualFiltering(ctx, logger, limit, filterGroup)
	}

	// Execute the search with the filter
	searchResult, err := client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: "content source topic year _additional { certainty id }",
		}).
		WithNearVector(client.GraphQL().NearVectorArgBuilder().
			WithVector(queryVector)).
		WithWhere(whereFilter).
		WithLimit(limit).
		Do(ctx)

	if err != nil {
		logger.Warn(ctx, "Search with Weaviate filter failed, falling back to manual filtering", map[string]interface{}{
			"error": err.Error(),
		})
		return searchWithManualFiltering(ctx, logger, limit, filterGroup)
	}

	// Process search results
	var searchResults []interfaces.SearchResult
	if searchResult != nil && searchResult.Data != nil {
		if getMap, ok := searchResult.Data["Get"].(map[string]interface{}); ok {
			if resultsList, ok := getMap[className].([]interface{}); ok {
				logger.Info(ctx, fmt.Sprintf("Found %d search results with Weaviate filter", len(resultsList)), nil)

				// Convert to SearchResult format
				searchResults = make([]interfaces.SearchResult, 0, len(resultsList))

				for _, result := range resultsList {
					if resultMap, ok := result.(map[string]interface{}); ok {
						content, _ := resultMap["content"].(string)
						additional, _ := resultMap["_additional"].(map[string]interface{})

						var certainty float64
						var id string

						if additional != nil {
							certainty, _ = additional["certainty"].(float64)
							id, _ = additional["id"].(string)
						}

						// Create document
						doc := interfaces.Document{
							ID:       id,
							Content:  content,
							Metadata: make(map[string]interface{}),
						}

						// Extract metadata
						for k, v := range resultMap {
							if k != "content" && k != "_additional" {
								doc.Metadata[k] = v
							}
						}

						// Add to results
						searchResults = append(searchResults, interfaces.SearchResult{
							Document: doc,
							Score:    float32(certainty),
						})
					}
				}

				return searchResults, nil
			}
		}
	}

	// If we couldn't process the results, fall back to manual filtering
	logger.Warn(ctx, "Failed to process search results, falling back to manual filtering", nil)
	return searchWithManualFiltering(ctx, logger, limit, filterGroup)
}

// createSimpleFilter creates a simple Weaviate filter for a single field
func createSimpleFilter(filter embedding.MetadataFilter) *filters.WhereBuilder {
	// Create a filter for a single field
	whereBuilder := filters.Where().WithPath([]string{filter.Field})

	// Apply the appropriate operator
	switch strings.ToLower(filter.Operator) {
	case "=", "==", "eq":
		return whereBuilder.WithOperator(filters.Equal).WithValueString(fmt.Sprint(filter.Value))
	case "!=", "<>", "ne":
		return whereBuilder.WithOperator(filters.NotEqual).WithValueString(fmt.Sprint(filter.Value))
	case ">", "gt":
		return whereBuilder.WithOperator(filters.GreaterThan).WithValueNumber(toFloat64(filter.Value))
	case ">=", "gte":
		return whereBuilder.WithOperator(filters.GreaterThanEqual).WithValueNumber(toFloat64(filter.Value))
	case "<", "lt":
		return whereBuilder.WithOperator(filters.LessThan).WithValueNumber(toFloat64(filter.Value))
	case "<=", "lte":
		return whereBuilder.WithOperator(filters.LessThanEqual).WithValueNumber(toFloat64(filter.Value))
	case "contains":
		return whereBuilder.WithOperator(filters.Like).WithValueString(fmt.Sprint(filter.Value))
	default:
		// Default to equals for unknown operators
		return whereBuilder.WithOperator(filters.Equal).WithValueString(fmt.Sprint(filter.Value))
	}
}

// createComplexFilter creates a complex Weaviate filter with AND/OR logic
func createComplexFilter(ctx context.Context, logger logging.Logger, group embedding.MetadataFilterGroup) *filters.WhereBuilder {
	// Determine the operator
	var op filters.WhereOperator
	if strings.ToLower(group.Operator) == "or" {
		op = filters.Or
	} else {
		op = filters.And // Default to AND
	}

	// Create operands for each filter and subgroup
	var operands []*filters.WhereBuilder

	// Add filters
	for _, filter := range group.Filters {
		operands = append(operands, createSimpleFilter(filter))
	}

	// Add subgroups
	for _, subGroup := range group.SubGroups {
		subFilter := createComplexFilter(ctx, logger, subGroup)
		if subFilter != nil {
			operands = append(operands, subFilter)
		}
	}

	// If we have operands, create the complex filter
	if len(operands) > 0 {
		return filters.Where().WithOperator(op).WithOperands(operands)
	}

	// If no operands, return nil
	return nil
}

// toFloat64 converts a value to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// searchWithManualFiltering gets all documents and applies filters manually
func searchWithManualFiltering(ctx context.Context, logger logging.Logger,
	limit int, filterGroup embedding.MetadataFilterGroup) ([]interfaces.SearchResult, error) {

	logger.Info(ctx, "Performing manual filtering as fallback", nil)

	// First try to get all documents
	// Create a robust client
	client, className, err := createRobustWeaviateClient(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create robust client: %w", err)
	}

	// Retrieve all objects
	getAllResult, err := client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: "content source topic year _additional { id }",
		}).
		WithLimit(100). // Get a reasonable number of objects
		Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve all objects: %w", err)
	}

	// Extract all objects and convert to documents
	var allDocs []interfaces.Document
	if getAllResult != nil && getAllResult.Data != nil {
		if getMap, ok := getAllResult.Data["Get"].(map[string]interface{}); ok {
			if resultsList, ok := getMap[className].([]interface{}); ok {
				logger.Info(ctx, fmt.Sprintf("Retrieved %d objects for manual filtering", len(resultsList)), nil)

				for _, result := range resultsList {
					if resultMap, ok := result.(map[string]interface{}); ok {
						content, _ := resultMap["content"].(string)
						additional, _ := resultMap["_additional"].(map[string]interface{})

						var id = ""
						if additional != nil {
							id, _ = additional["id"].(string)
						}

						// Create document
						doc := interfaces.Document{
							ID:       id,
							Content:  content,
							Metadata: make(map[string]interface{}),
						}

						// Extract metadata
						for k, v := range resultMap {
							if k != "content" && k != "_additional" {
								doc.Metadata[k] = v
							}
						}

						allDocs = append(allDocs, doc)
					}
				}
			}
		}
	}

	if len(allDocs) == 0 {
		return nil, fmt.Errorf("no documents retrieved for manual filtering")
	}

	// Apply filters manually
	filteredDocs := embedding.ApplyFilters(allDocs, filterGroup)
	logger.Info(ctx, fmt.Sprintf("Manual filtering resulted in %d documents", len(filteredDocs)), nil)

	// Convert to search results
	results := convertToSearchResults(filteredDocs)

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// searchWithVectorAndWeaviateFilters performs a vector search using the Weaviate filter format
func searchWithVectorAndWeaviateFilters(ctx context.Context, logger logging.Logger,
	embedder embedding.Client, query string, limit int, filterGroup embedding.MetadataFilterGroup) ([]interfaces.SearchResult, error) {

	// Generate embedding for the query
	vector, err := embedder.Embed(ctx, query)
	if err != nil {
		logger.Error(ctx, "Failed to generate embedding for search", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, err
	}

	// Create a direct Weaviate filter instead of trying to convert from a map
	var whereFilter *filters.WhereBuilder

	// Check if we have a simple filter for a single field
	if len(filterGroup.Filters) == 1 && len(filterGroup.SubGroups) == 0 {
		// Simple case: single filter
		filter := filterGroup.Filters[0]
		whereFilter = createSimpleFilter(filter)
	} else if strings.ToLower(filterGroup.Operator) == "and" || strings.ToLower(filterGroup.Operator) == "or" {
		// Complex case: AND/OR with multiple conditions
		whereFilter = createComplexFilter(ctx, logger, filterGroup)
	}

	// If we couldn't create a filter, fall back to manual filtering
	if whereFilter == nil {
		logger.Warn(ctx, "Failed to create Weaviate filter, falling back to manual filtering", nil)
		return searchWithManualFiltering(ctx, logger, limit, filterGroup)
	}

	logger.Info(ctx, "Using direct Weaviate filter format for vector search", map[string]interface{}{
		"filter": "direct filter builder used",
	})

	// Create a robust client
	client, className, err := createRobustWeaviateClient(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create robust client: %w", err)
	}

	// Execute the search with the filter
	searchResult, err := client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: "content source topic year _additional { certainty id }",
		}).
		WithNearVector(client.GraphQL().NearVectorArgBuilder().
			WithVector(vector)).
		WithWhere(whereFilter).
		WithLimit(limit).
		Do(ctx)

	if err != nil {
		logger.Warn(ctx, "Vector search with Weaviate filter failed, falling back to manual filtering", map[string]interface{}{
			"error": err.Error(),
		})
		return searchWithManualFiltering(ctx, logger, limit, filterGroup)
	}

	// Process search results
	var searchResults []interfaces.SearchResult
	if searchResult != nil && searchResult.Data != nil {
		if getMap, ok := searchResult.Data["Get"].(map[string]interface{}); ok {
			if resultsList, ok := getMap[className].([]interface{}); ok {
				logger.Info(ctx, fmt.Sprintf("Found %d vector search results with Weaviate filter", len(resultsList)), nil)

				// Convert to SearchResult format
				searchResults = make([]interfaces.SearchResult, 0, len(resultsList))

				for _, result := range resultsList {
					if resultMap, ok := result.(map[string]interface{}); ok {
						content, _ := resultMap["content"].(string)
						additional, _ := resultMap["_additional"].(map[string]interface{})

						var certainty float64
						var id string

						if additional != nil {
							certainty, _ = additional["certainty"].(float64)
							id, _ = additional["id"].(string)
						}

						// Create document
						doc := interfaces.Document{
							ID:       id,
							Content:  content,
							Metadata: make(map[string]interface{}),
						}

						// Extract metadata
						for k, v := range resultMap {
							if k != "content" && k != "_additional" {
								doc.Metadata[k] = v
							}
						}

						// Add to results
						searchResults = append(searchResults, interfaces.SearchResult{
							Document: doc,
							Score:    float32(certainty),
						})
					}
				}

				return searchResults, nil
			}
		}
	}

	// If we couldn't process the results, fall back to manual filtering
	logger.Warn(ctx, "Failed to process vector search results, falling back to manual filtering", nil)
	return searchWithManualFiltering(ctx, logger, limit, filterGroup)
}

// searchWithDirectWeaviateFilter demonstrates using the Weaviate filter builder directly
func searchWithDirectWeaviateFilter(ctx context.Context, logger logging.Logger, query string, limit int) {
	logger.Info(ctx, "Performing search with direct Weaviate filter builder...", nil)

	// Create a robust client
	client, className, err := createRobustWeaviateClient(ctx, logger)
	if err != nil {
		logger.Error(ctx, "Failed to create robust client", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Generate embedding for the query
	cfg := config.Get()
	embedder := embedding.NewOpenAIEmbedderWithConfig(cfg.LLM.OpenAI.APIKey, embedding.DefaultEmbeddingConfig(cfg.LLM.OpenAI.EmbeddingModel))

	queryVector, err := embedder.Embed(ctx, query)
	if err != nil {
		logger.Error(ctx, "Failed to generate embedding for query", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Create a complex filter using Weaviate's filter builder directly
	// Year > 2020
	yearFilter := filters.Where().
		WithPath([]string{"year"}).
		WithOperator(filters.GreaterThan).
		WithValueNumber(2020)

	// Source = wikipedia
	wikipediaFilter := filters.Where().
		WithPath([]string{"source"}).
		WithOperator(filters.Equal).
		WithValueString("wikipedia")

	// Source = textbook
	textbookFilter := filters.Where().
		WithPath([]string{"source"}).
		WithOperator(filters.Equal).
		WithValueString("textbook")

	// (Source = wikipedia OR Source = textbook)
	sourceFilter := filters.Where().
		WithOperator(filters.Or).
		WithOperands([]*filters.WhereBuilder{wikipediaFilter, textbookFilter})

	// Year > 2020 AND (Source = wikipedia OR Source = textbook)
	complexFilter := filters.Where().
		WithOperator(filters.And).
		WithOperands([]*filters.WhereBuilder{yearFilter, sourceFilter})

	logger.Info(ctx, "Executing direct vector search with complex filter", nil)
	searchResult, err := client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: "content source topic year _additional { certainty id }",
		}).
		WithNearVector(client.GraphQL().NearVectorArgBuilder().
			WithVector(queryVector)).
		WithWhere(complexFilter).
		WithLimit(limit).
		Do(ctx)

	if err != nil {
		logger.Error(ctx, "Direct vector search with complex filter failed", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Debug the response
	debugGraphQLResponse(ctx, logger, searchResult)

	// Process vector search results
	if searchResult != nil && searchResult.Data != nil {
		logger.Info(ctx, "Direct vector search with complex filter succeeded", nil)

		// Try to access the data
		if getMap, ok := searchResult.Data["Get"].(map[string]interface{}); ok {
			if resultsList, ok := getMap[className].([]interface{}); ok {
				logger.Info(ctx, fmt.Sprintf("Found %d vector search results with complex filter", len(resultsList)), nil)

				// Convert to SearchResult format
				searchResults := make([]interfaces.SearchResult, 0, len(resultsList))

				for _, result := range resultsList {
					if resultMap, ok := result.(map[string]interface{}); ok {
						content, _ := resultMap["content"].(string)
						additional, _ := resultMap["_additional"].(map[string]interface{})

						var certainty float64
						var id string

						if additional != nil {
							certainty, _ = additional["certainty"].(float64)
							id, _ = additional["id"].(string)
						}

						// Create document
						doc := interfaces.Document{
							ID:       id,
							Content:  content,
							Metadata: make(map[string]interface{}),
						}

						// Extract metadata
						for k, v := range resultMap {
							if k != "content" && k != "_additional" {
								doc.Metadata[k] = v
							}
						}

						// Add to results
						searchResults = append(searchResults, interfaces.SearchResult{
							Document: doc,
							Score:    float32(certainty),
						})
					}
				}

				// Print the results
				logger.Info(ctx, "Search results using direct Weaviate filter builder:", nil)
				printResults(ctx, logger, searchResults)
				return
			}
		}
	}

	logger.Warn(ctx, "No search results found with complex filter", nil)
}

// validateWeaviateConnection checks if Weaviate is accessible
func validateWeaviateConnection(url string) error {
	// Simple HTTP check to see if Weaviate is reachable
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("received status code %d from Weaviate", resp.StatusCode)
	}

	return nil
}

// calculateCosineSimilarity calculates the cosine similarity between two vectors
func calculateCosineSimilarity(vec1, vec2 []float32) (float64, error) {
	if len(vec1) != len(vec2) {
		return 0, fmt.Errorf("vectors must have the same dimensions: %d != %d", len(vec1), len(vec2))
	}

	var dotProduct, magnitude1, magnitude2 float64
	for i := 0; i < len(vec1); i++ {
		dotProduct += float64(vec1[i] * vec2[i])
		magnitude1 += float64(vec1[i] * vec1[i])
		magnitude2 += float64(vec2[i] * vec2[i])
	}

	magnitude1 = float64(math.Sqrt(magnitude1))
	magnitude2 = float64(math.Sqrt(magnitude2))

	if magnitude1 == 0 || magnitude2 == 0 {
		return 0, fmt.Errorf("vector magnitude is zero")
	}

	return dotProduct / (magnitude1 * magnitude2), nil
}

// validateDocumentsStored checks if documents were properly stored
func validateDocumentsStored(ctx context.Context, logger logging.Logger, store interfaces.VectorStore, documents []interfaces.Document) bool {
	logger.Info(ctx, "Validating document storage by retrieving sample documents...", nil)

	// Get a sample of document IDs to validate
	sampleSize := 3
	if len(documents) < sampleSize {
		sampleSize = len(documents)
	}

	sampleIDs := make([]string, sampleSize)
	for i := 0; i < sampleSize; i++ {
		sampleIDs[i] = documents[i].ID
	}

	// Try to retrieve the documents
	retrievedDocs, err := store.Get(ctx, sampleIDs)
	if err != nil {
		logger.Warn(ctx, "Error retrieving documents for validation", map[string]interface{}{
			"error": err.Error(),
		})
		return false
	}

	logger.Info(ctx, fmt.Sprintf("Successfully validated document storage retrieved_count=%d", len(retrievedDocs)), nil)

	// Check if we got all the documents we requested
	return len(retrievedDocs) == sampleSize
}

// createRobustWeaviateClient creates a Weaviate client with robust error handling
func createRobustWeaviateClient(ctx context.Context, logger logging.Logger) (*weaviate.Client, string, error) {
	// Get configuration
	cfg := config.Get()

	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Create class name with organization ID
	className := fmt.Sprintf("Example_%s", orgID)

	// Log connection details
	logger.Debug(ctx, "Connecting to Weaviate", map[string]interface{}{
		"host":            cfg.VectorStore.Weaviate.Host,
		"scheme":          cfg.VectorStore.Weaviate.Scheme,
		"has_api_key":     cfg.VectorStore.Weaviate.APIKey != "",
		"has_openai_key":  cfg.LLM.OpenAI.APIKey != "",
		"organization_id": orgID,
	})

	// Create Weaviate client configuration
	clientConfig := weaviate.Config{
		Host:   cfg.VectorStore.Weaviate.Host,
		Scheme: cfg.VectorStore.Weaviate.Scheme,
	}

	// Add API key if provided
	if cfg.VectorStore.Weaviate.APIKey != "" {
		clientConfig.Headers = map[string]string{
			"Authorization": "Bearer " + cfg.VectorStore.Weaviate.APIKey,
		}
	}

	// Create client
	client, err := weaviate.NewClient(clientConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create Weaviate client: %w", err)
	}

	// Test the client with a simple query
	logger.Info(ctx, "Testing Weaviate client with a simple query...", nil)
	_, err = client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: "_additional { id }",
		}).
		WithLimit(1).
		Do(ctx)

	if err != nil {
		return nil, "", fmt.Errorf("failed to test Weaviate client: %w", err)
	}

	logger.Info(ctx, "Test query succeeded", nil)

	return client, className, nil
}

// debugGraphQLResponse logs details about a GraphQL response for debugging
func debugGraphQLResponse(ctx context.Context, logger logging.Logger, result interface{}) {
	logger.Debug(ctx, "GraphQL response type", map[string]interface{}{
		"type": fmt.Sprintf("%T", result),
	})

	// Check if it's a GraphQL response with Data field
	if resp, ok := result.(interface{ GetData() map[string]interface{} }); ok {
		data := resp.GetData()
		hasData := data != nil
		logger.Debug(ctx, "GraphQL response data", map[string]interface{}{
			"has_data": hasData,
			"errors":   nil, // We don't have access to errors field generically
		})

		if hasData {
			// Log the keys in the data map
			keys := make([]string, 0, len(data))
			for k, v := range data {
				keys = append(keys, k)
				logger.Debug(ctx, fmt.Sprintf("GraphQL data key: %s", k), map[string]interface{}{
					"value_type": fmt.Sprintf("%T", v),
				})
			}
			logger.Debug(ctx, "GraphQL data keys", map[string]interface{}{
				"keys": keys,
			})
		}
	}
}

// tryGetAllDocuments attempts to retrieve all documents by their IDs
func tryGetAllDocuments(ctx context.Context, logger logging.Logger, store interfaces.VectorStore, documents []interfaces.Document) {
	// Get a sample of document IDs to retrieve
	sampleSize := 5
	if len(documents) < sampleSize {
		sampleSize = len(documents)
	}

	sampleIDs := make([]string, sampleSize)
	for i := 0; i < sampleSize; i++ {
		sampleIDs[i] = documents[i].ID
	}

	logger.Debug(ctx, "Attempting to retrieve documents by ID", map[string]interface{}{
		"document_count": len(sampleIDs),
		"document_ids":   sampleIDs,
	})

	// Try to retrieve the documents
	logger.Info(ctx, "Getting all documents by ID...", nil)
	retrievedDocs, err := store.Get(ctx, sampleIDs)
	if err != nil {
		logger.Error(ctx, "Error retrieving documents", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	logger.Info(ctx, fmt.Sprintf("Retrieved %d documents", len(retrievedDocs)), nil)

	// Log document details
	for i, doc := range retrievedDocs {
		if i < 3 {
			logger.Info(ctx, fmt.Sprintf("Document %d: %s", i+1, truncateString(doc.Content, 100)), nil)
			logger.Info(ctx, fmt.Sprintf("    ID: %s", doc.ID), nil)
			logger.Info(ctx, fmt.Sprintf("    Metadata: %v", doc.Metadata), nil)
		}
	}

	if len(retrievedDocs) > 3 {
		logger.Info(ctx, fmt.Sprintf("... %d more documents omitted ...", len(retrievedDocs)-3), nil)
	}
}
