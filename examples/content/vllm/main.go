// Command vllm sends a file and a question to an agent backed by vLLM.
//
//	VLLM_BASE_URL=http://localhost:8000 VLLM_MODEL=Qwen/Qwen2.5-VL-7B-Instruct go run ./examples/content/vllm photo.jpg "Describe this photo."
//
// It takes images, as a local file or a URL, and sends them through the chat endpoint. The served model
// must accept images.
package main

import (
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
)

func main() {
	file, question := contentdemo.Args("vllm")
	llm := vllm.NewClient(
		vllm.WithBaseURL(contentdemo.Env("VLLM_BASE_URL")),
		vllm.WithModel(contentdemo.Env("VLLM_MODEL")),
	)

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
