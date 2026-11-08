// Command gemini sends a file and a question to an agent backed by Gemini.
//
//	GEMINI_API_KEY=... go run ./examples/content/gemini talk.mp3 "List the main points of this talk."
//
// It sends any part as is, and the Gemini API checks the MIME type. A URL must be a Files API URI or a
// public HTTPS URL.
package main

import (
	"context"
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
)

func main() {
	file, question := contentdemo.Args("gemini")
	llm, err := gemini.NewClient(context.Background(),
		gemini.WithAPIKey(contentdemo.Env("GEMINI_API_KEY")),
		gemini.WithModel(gemini.ModelGemini35Flash),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
