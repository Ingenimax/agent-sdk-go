// Command ollama sends a file and a question to an agent backed by Ollama.
//
//	OLLAMA_BASE_URL=http://localhost:11434 OLLAMA_MODEL=llava go run ./examples/content/ollama photo.jpg "Describe this photo."
//
// It takes images as a local file. The model must accept images, such as llava.
package main

import (
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
)

func main() {
	file, question := contentdemo.Args("ollama")
	llm := ollama.NewClient(
		ollama.WithBaseURL(contentdemo.Env("OLLAMA_BASE_URL")),
		ollama.WithModel(contentdemo.Env("OLLAMA_MODEL")),
	)

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
