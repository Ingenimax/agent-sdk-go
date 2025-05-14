### Memory Example
This example demonstrates different memory implementations in the Agent SDK. Memory is a crucial component for agents to maintain context across interactions.

## Prerequisites
Before running the example, you'll need:
1. An OpenAI API key (for the Conversation Summary Memory)
2. Redis running locally (for the Redis Memory)
3. Weaviate running locally (for the Vector Store Retriever Memory)

## Setup
1. Set environment variables:
```bash
# Required for Conversation Summary Memory
export OPENAI_API_KEY=your_openai_api_key

# Optional for Redis Memory (defaults to localhost:6379)
export REDIS_ADDR=your_redis_address

# Required for Weaviate Vector Store
export WEAVIATE_URL=http://localhost:8080
export WEAVIATE_API_KEY=your_weaviate_api_key  # If authentication is enabled
```
2. Start Redis:

```bash
docker run -d --name redis-stack -p 6379:6379 redis/redis-stack-server:latest
```

3. Start Weaviate:

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
go build -o memory_example cmd/examples/memory/main.go
```

## Memory Types Demonstrated

### 1. Conversation Buffer Memory

A simple in-memory buffer that stores conversation messages. Features:
- Configurable maximum size
- Filtering by role
- Limiting the number of returned messages

### 2. Conversation Summary Memory

Summarizes older messages to maintain context while keeping memory usage low. Features:
- Uses an LLM to generate summaries
- Configurable buffer size before summarization
- Maintains important context while reducing token usage

### 3. Vector Store Retriever Memory

Stores messages in a vector database for semantic retrieval. Features:
- Semantic search capabilities
- Efficient storage of large conversation histories
- Retrieval based on relevance to current context

### 4. Redis Memory

Persists conversation history in Redis. Features:
- Persistent storage across sessions
- Configurable time-to-live (TTL)
- Distributed access to conversation history

## Example Output

When you run the example, you'll see output demonstrating each memory type:

```
=== Conversation Buffer Memory ===
All messages:
1. system: You are a helpful assistant.
2. user: Hello, how are you?
3. assistant: I'm doing well, thank you for asking! How can I help you today?
4. user: Tell me about the weather.

User messages only:
1. user: Hello, how are you?
2. user: Tell me about the weather.

Last 2 messages:
1. assistant: I'm doing well, thank you for asking! How can I help you today?
2. user: Tell me about the weather.

After clearing:
Memory cleared successfully

=== Conversation Summary Memory ===
...similar output with summarization...

=== Vector Store Retriever Memory ===
...similar output with vector storage...

=== Redis Memory ===
...similar output with Redis storage...
```

## Customization

You can customize the memory implementations by:

1. Adjusting buffer sizes
2. Changing the LLM model for summarization
3. Implementing different vector stores
4. Configuring Redis options like TTL

## Troubleshooting

If you encounter issues:

1. Ensure your OpenAI API key is valid
2. Check that Redis and Weaviate are running and accessible
3. Look for error messages indicating missing dependencies
4. Verify that the conversation and organization IDs are set in the context
