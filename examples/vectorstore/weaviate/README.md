### Weaviate Vector Store Example
This example demonstrates how to use Weaviate as a vector store with the Agent SDK. It shows basic operations like storing, searching, and deleting documents.

## Prerequisites
Before running the example, you'll need:
1. An OpenAI API key (for text embeddings)
2. Weaviate running locally or in the cloud

## Setup

Set environment variables:
```bash
# Required for OpenAI embeddings
export OPENAI_API_KEY=your_openai_api_key
# Optional: Set custom embedding model (defaults to text-embedding-3-small)
export OPENAI_EMBEDDING_MODEL=text-embedding-3-small

# Weaviate connection details
export WEAVIATE_HOST=localhost:8080
export WEAVIATE_API_KEY=your_weaviate_api_key  # If authentication is enabled
export WEAVIATE_SCHEME=http  # Use https for cloud instances
```

2. Start Weaviate:

```bash
docker run -d --name weaviate \
  -p 8080:8080 \
  -e QUERY_DEFAULTS_LIMIT=25 \
  -e AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED=true \
  -e PERSISTENCE_DATA_PATH='/var/lib/weaviate' \
  -e DEFAULT_VECTORIZER_MODULE=text2vec-openai \
  -e ENABLE_MODULES=text2vec-openai \
  -e OPENAI_APIKEY=$OPENAI_API_KEY \
  semitechnologies/weaviate:1.19.6
```

## Running the Example

Run the compiled binary:

```bash
go build -o weaviate_example examples/vectorstore/weaviate/main.go
./weaviate_example
```

## Example Code

The example demonstrates:

1. Connecting to Weaviate
2. Storing documents with metadata
3. Searching for similar documents
4. Dynamic field selection (auto-discovery vs specific fields)
5. Filtering search results by metadata
6. Vector-based search
7. Deleting documents

## Weaviate Schema and Field Management

This vector store implementation provides **dynamic field discovery** for flexible data retrieval:

### 🚀 **How It Works**

1. **Collection Creation**: Weaviate creates collections automatically when you store the first document
2. **Dynamic Property Addition**: New metadata fields are automatically added to the schema as needed
3. **Smart Type Inference**: Weaviate automatically detects optimal data types:
   - `string` → `text`
   - `int/int64` → `int`
   - `float32/float64` → `number`
   - `bool` → `boolean`
   - `[]interface{}` → `text[]` (arrays)
   - `map[string]interface{}` → `object`
4. **Field Discovery**: The implementation can automatically discover all available fields from the schema

### 💡 **Benefits**

- ✅ **Zero setup** - just start storing documents
- ✅ **Automatic adaptation** - schema evolves with your data
- ✅ **Type safety** - Weaviate validates data types automatically
- ✅ **Performance optimized** - Weaviate chooses optimal settings
- ✅ **Production ready** - Built and tested by Weaviate team

### 📖 **Usage Examples**

```go
// Simple storage - Weaviate handles schema automatically
docs := []interfaces.Document{
    {
        ID: "1",
        Content: "The quick brown fox jumps over the lazy dog",
        Metadata: map[string]interface{}{
            "source": "example",           // → text
            "wordCount": 9,               // → int
            "isClassic": true,            // → boolean
            "rating": 4.8,                // → number
            "tags": []string{"pangram"},  // → text[]
        },
    },
}

// Weaviate creates collection and properties automatically
err := store.Store(ctx, docs)
```

## Dynamic Field Selection

The Weaviate vector store supports dynamic field selection for search operations. This allows you to:

1. **Auto-discovery (default)**: Automatically retrieve all fields from the schema without hardcoding field names
2. **Specific field selection**: Choose only the fields you need to reduce payload size and improve performance
3. **Graceful fallback**: Automatically falls back to basic fields if schema discovery fails

### Usage Examples

```go
// Auto-discovery: Gets all fields dynamically from schema
results, err := store.Search(ctx, "fox jumps", 5)

// Specific fields: Only retrieve content and source fields
results, err := store.Search(ctx, "fox jumps", 5,
    interfaces.WithFields("content", "source"))

// Minimal fields: Just content for lightweight responses
results, err := store.Search(ctx, "fox jumps", 5,
    interfaces.WithFields("content"))

// Filtering: Search with metadata filters
results, err := store.Search(ctx, "fox jumps", 5,
    interfaces.WithFilters(map[string]interface{}{
        "isClassic": true,
    }))
```

### 🔍 **Advanced Filtering**

The Weaviate vector store supports multiple filtering options:

#### Simple Equality Filters
```go
// Simple key-value equality
results, err := store.Search(ctx, "query", 5,
    interfaces.WithFilters(map[string]interface{}{
        "isClassic": true,
        "source": "example",
    }))
```

#### Advanced Filtering with Operators
```go
// Using operators for complex conditions
results, err := store.Search(ctx, "query", 5,
    interfaces.WithFilters(map[string]interface{}{
        "rating": map[string]interface{}{
            "operator": "greaterThan",
            "value": 4.0,
        },
        "source": map[string]interface{}{
            "operator": "contains",
            "value": "example",
        },
    }))
```

#### Logical Operators (AND/OR)
```go
// AND conditions
results, err := store.Search(ctx, "query", 5,
    interfaces.WithFilters(map[string]interface{}{
        "and": []interface{}{
            map[string]interface{}{"isClassic": true},
            map[string]interface{}{
                "rating": map[string]interface{}{
                    "operator": "greaterThan",
                    "value": 4.0,
                },
            },
        },
    }))

// OR conditions
results, err := store.Search(ctx, "query", 5,
    interfaces.WithFilters(map[string]interface{}{
        "or": []interface{}{
            map[string]interface{}{"source": "example"},
            map[string]interface{}{"source": "literature"},
        },
    }))
```

#### Supported Operators
- `equals` / `notEquals` - String and number equality
- `greaterThan` / `greaterThanEqual` - Number comparisons
- `lessThan` / `lessThanEqual` - Number comparisons
- `like` / `contains` - String pattern matching
- `in` - Array containment (for array fields)

### 🔧 **Implementation Details**

- **Backward compatibility**: Existing code continues to work without changes
- **Error handling**: Graceful fallback to basic fields if schema discovery fails
- **Performance**: Field discovery is cached and optimized
- **Multi-tenancy**: Supports tenant-based operations with proper field isolation
