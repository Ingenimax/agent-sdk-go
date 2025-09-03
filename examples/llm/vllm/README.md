# vLLM Examples

This directory contains examples demonstrating how to use the vLLM LLM provider with the Agent SDK.

## Overview

vLLM is a fast and efficient library for LLM inference and serving, designed for high-performance, low-latency inference of large language models. These examples show how to integrate vLLM with the Agent SDK for various use cases.

## Examples Included

### 1. Basic Usage (`main.go`)

Demonstrates basic vLLM client usage including:
- Text generation with different temperature settings
- Chat conversations with message history
- Multi-turn conversations
- Model management (listing available models)
- Temperature variations
- System message variations
- Performance testing with multiple requests

### 2. Structured Output (`structured_output/main.go`)

Shows how to use vLLM with structured output capabilities:
- Automatic schema generation from Go structs
- Person information extraction with nested data
- Weather information with structured fields
- Programming task analysis with complex structures
- Agent integration with structured output
- Error handling for invalid JSON responses
- Complex nested structures

### 3. Agent Integration (`agent_integration/main.go`)

Demonstrates vLLM integration with the Agent framework:
- Basic conversation with memory
- Follow-up conversations testing memory retention
- Technical questions and code generation
- Complex problem solving
- Different system prompts for specialized agents
- Memory persistence testing
- Performance comparison with multiple requests
- Error handling for long prompts
- Model information and configuration

## Prerequisites

1. **Install vLLM**: Follow the installation instructions at [vLLM GitHub](https://github.com/vllm-project/vllm)
2. **Start vLLM Server**: Run vLLM server with your model
3. **Model Files**: Ensure your model is available for vLLM

## Quick Start

### 1. Start vLLM Server

```bash
# Install vLLM
pip install vllm

# Start server with a model
python -m vllm.entrypoints.openai.api_server \
    --model llama-2-7b \
    --host 0.0.0.0 \
    --port 8000
```

### 2. Run Examples

```bash
# Basic usage example
go run examples/llm/vllm/main.go

# Structured output example
go run examples/llm/vllm/structured_output/main.go

# Agent integration example
go run examples/llm/vllm/agent_integration/main.go
```

## Configuration

### Environment Variables

You can configure the examples using environment variables:

```bash
export VLLM_BASE_URL=http://localhost:8000
export VLLM_MODEL=llama-2-7b
```

### Default Settings

- **Base URL**: `http://localhost:8000`
- **Model**: `llama-2-7b`
- **Timeout**: 60 seconds
- **Retry**: 3 attempts with exponential backoff

## Features Demonstrated

### Basic Client Features

- **Text Generation**: Simple prompt-to-text generation
- **Chat Completions**: Multi-turn conversations with message history
- **Model Management**: List available models
- **Temperature Control**: Adjust randomness in responses
- **System Messages**: Set context and behavior
- **Error Handling**: Comprehensive error handling and retry logic

### Structured Output Features

- **Automatic Schema Generation**: Use `structuredoutput.NewResponseFormat()` to generate JSON schemas from Go structs
- **Type Safety**: Direct unmarshaling into Go structs
- **Field Descriptions**: Use struct tags to provide field descriptions
- **Required/Optional Fields**: Use `omitempty` tag for optional fields
- **Nested Structures**: Support for complex nested objects and arrays

### Agent Integration Features

- **Memory Management**: Conversation memory with persistence
- **System Prompts**: Specialized agent personalities
- **Multi-turn Conversations**: Context-aware responses
- **Error Handling**: Robust error handling for various scenarios
- **Performance Testing**: Multiple concurrent requests

## Supported Models

vLLM supports many models. Some popular ones include:

- `llama-2-7b`: Meta's Llama 2 7B model
- `llama-2-13b`: Meta's Llama 2 13B model
- `llama-2-70b`: Meta's Llama 2 70B model
- `mistral-7b`: Mistral AI's 7B model
- `codellama-7b`: Code-focused Llama model
- `vicuna-7b`: Vicuna fine-tuned model

## Performance Considerations

- **PagedAttention**: vLLM uses PagedAttention for efficient memory management
- **GPU Optimization**: Optimized for GPU inference with CUDA
- **Batch Processing**: Efficient handling of multiple requests
- **Memory Efficiency**: Better memory usage than traditional inference
- **Low Latency**: Designed for high-throughput, low-latency inference

## Troubleshooting

### Common Issues

1. **Connection Refused**: Ensure the vLLM server is running on the correct port
2. **Model Not Found**: Check if the model is loaded using `ListModels()`
3. **Out of Memory**: Reduce batch size or use smaller models
4. **Slow Response**: Check GPU utilization and model size

### Debug Mode

Enable debug logging to troubleshoot issues:

```go
logger := logging.New()
logger.SetLevel(logging.DebugLevel)

client := vllm.NewClient(
    vllm.WithLogger(logger),
)
```

### Performance Tuning

- **GPU Memory**: Adjust `--gpu-memory-utilization` based on your GPU
- **Batch Size**: Optimize batch size for your use case
- **Model Size**: Use smaller models for faster inference
- **Tensor Parallel**: Use multiple GPUs for large models

## Comparison with Other Providers

| Feature | vLLM | Ollama | OpenAI | VertexAI |
|---------|------|--------|--------|----------|
| Local Inference | ✅ | ✅ | ❌ | ❌ |
| Performance | Very High | Medium | High | High |
| Memory Efficiency | Very High | Medium | High | High |
| Model Management | ✅ | ✅ | ❌ | ❌ |
| Structured Output | ✅ | ✅ | ✅ | ✅ |
| Tool Integration | Basic | Basic | Full | Full |
| Cost | Low | Low | High | High |
| GPU Optimization | Excellent | Good | N/A | N/A |

## Advanced Configuration

### Starting vLLM with Advanced Settings

```bash
python -m vllm.entrypoints.openai.api_server \
    --model llama-2-7b \
    --host 0.0.0.0 \
    --port 8000 \
    --tensor-parallel-size 2 \
    --gpu-memory-utilization 0.9 \
    --max-model-len 4096
```

### Using Different Models

```bash
# Use a different model
export VLLM_MODEL=mistral-7b
go run examples/llm/vllm/main.go

# Use a code-focused model
export VLLM_MODEL=codellama-7b
go run examples/llm/vllm/main.go
```

## Contributing

When contributing to the vLLM examples:

1. Follow the existing code patterns
2. Add comprehensive error handling
3. Include performance considerations
4. Test with multiple model types
5. Consider vLLM-specific optimizations
6. Update documentation for new features
