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

## Weaviate Auto-Schema

This vector store leverages **Weaviate's native auto-schema capabilities** for maximum simplicity and reliability:

### 🚀 **How It Works**

1. **Automatic Collection Creation**: Weaviate creates collections automatically when you store the first document
2. **Dynamic Property Addition**: New metadata fields are automatically added to the schema as needed
3. **Smart Type Inference**: Weaviate automatically detects optimal data types:
   - `string` → `text`
   - `int/int64` → `int`
   - `float32/float64` → `number` 
   - `bool` → `boolean`
   - `[]interface{}` → `text[]` (arrays)
   - `map[string]interface{}` → `object`
4. **Zero Configuration**: No manual schema definition required

### 💡 **Benefits**

- ✅ **Zero setup** - just start storing documents  
- ✅ **Automatic adaptation** - schema evolves with your data
- ✅ **Type safety** - Weaviate validates data types automatically
- ✅ **Performance optimized** - Weaviate chooses optimal settings
- ✅ **Production ready** - Built and tested by Weaviate team

### 📖 **Usage Examples**

```go
// Simple storage - Weaviate handles everything automatically
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

// Auto-schema creates collection and properties automatically
err := store.Store(ctx, docs)
```

## Dynamic Field Selection

The Weaviate vector store now supports dynamic field selection for search operations. This allows you to:

1. **Auto-discovery (default)**: Automatically retrieve all fields from the schema without hardcoding field names
2. **Specific field selection**: Choose only the fields you need to reduce payload size and improve performance
3. **Graceful fallback**: Automatically falls back to basic fields if schema discovery fails

### Usage Examples

```go
// Auto-discovery: Gets all fields dynamically from schema
results, err := store.Search(ctx, "fox jumps", 5, interfaces.WithEmbedding(true))

// Specific fields: Only retrieve content and source fields
results, err := store.Search(ctx, "fox jumps", 5, 
    interfaces.WithEmbedding(true),
    interfaces.WithFields([]string{"content", "source"}))

// Minimal fields: Just content for lightweight responses
results, err := store.Search(ctx, "fox jumps", 5,
    interfaces.WithEmbedding(true), 
    interfaces.WithFields([]string{"content"}))
```

- **Backward compatibility**: Existing code continues to work without changes
