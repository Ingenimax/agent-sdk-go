// Command anthropic sends a file and a question to an agent backed by Anthropic.
//
//	ANTHROPIC_API_KEY=... go run ./examples/content/anthropic report.pdf "Summarize this report."
//
// It takes JPEG, PNG, GIF, and WebP images and PDFs, as a local file or a URL.
package main

import (
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
)

func main() {
	file, question := contentdemo.Args("anthropic")
	llm := anthropic.NewClient(contentdemo.Env("ANTHROPIC_API_KEY"), anthropic.WithModel(anthropic.ClaudeSonnet45))

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
