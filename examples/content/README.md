# Content input examples

Each example sends a file and a question to an agent backed by one LLM
provider. It then does the following:

1. Sends the file and the question with `RunWithContent`, and prints the
   answer.
2. Asks a follow-up question with `Run`. The agent's memory stores the file
   with the first message, so the follow-up can refer to it.
3. Sends the same file and question to the LLM client directly, with
   `interfaces.WithParts`, and prints that answer too.

The file can be a local path or a URL. The example detects a local file's
MIME type from its content and takes a URL's MIME type from its extension. It
sets the file name as the display name, which some providers send.

The shared code is in [contentdemo](contentdemo/contentdemo.go). For what
each provider sends, see [Content input](../../docs/content-input.md).

## Run an example

Each example takes a file and a question. Set the environment variables it
needs, then run it from the repository root:

| Provider | Command |
| --- | --- |
| OpenAI | `OPENAI_API_KEY=... go run ./examples/content/openai chart.png "What trend does this chart show?"` |
| Azure OpenAI | `AZURE_OPENAI_API_KEY=... AZURE_OPENAI_BASE_URL=... AZURE_OPENAI_DEPLOYMENT=... go run ./examples/content/azureopenai chart.png "What trend does this chart show?"` |
| Anthropic | `ANTHROPIC_API_KEY=... go run ./examples/content/anthropic report.pdf "Summarize this report."` |
| Gemini | `GEMINI_API_KEY=... go run ./examples/content/gemini talk.mp3 "List the main points of this talk."` |
| Bedrock | `BEDROCK_API_KEY=... BEDROCK_MODEL=us.amazon.nova-pro-v1:0 go run ./examples/content/bedrock report.pdf "Summarize this report."` |
| Ollama | `OLLAMA_BASE_URL=http://localhost:11434 OLLAMA_MODEL=llava go run ./examples/content/ollama photo.jpg "Describe this photo."` |
| DeepSeek | `DEEPSEEK_API_KEY=... go run ./examples/content/deepseek chart.png "What trend does this chart show?"` |
| vLLM | `VLLM_BASE_URL=http://localhost:8000 VLLM_MODEL=Qwen/Qwen2.5-VL-7B-Instruct go run ./examples/content/vllm photo.jpg "Describe this photo."` |

The model must accept the kind of file you send. A provider that can't send
the file returns an error that wraps `interfaces.ErrContentUnsupported`.
