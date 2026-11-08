// Command openai sends a file and a question to an agent backed by OpenAI.
//
//	OPENAI_API_KEY=... go run ./examples/content/openai chart.png "What trend does this chart show?"
//
// It takes PNG, JPEG, WebP, and non-animated GIF images, as a local file or a URL, and PDFs as a local file.
package main

import (
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
)

func main() {
	file, question := contentdemo.Args("openai")
	llm := openai.NewClient(contentdemo.Env("OPENAI_API_KEY"), openai.WithModel("gpt-4o"))

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
