// Command azureopenai sends a file and a question to an agent backed by Azure OpenAI.
//
//	AZURE_OPENAI_API_KEY=... AZURE_OPENAI_BASE_URL=... AZURE_OPENAI_DEPLOYMENT=... go run ./examples/content/azureopenai chart.png "What trend does this chart show?"
//
// It takes PNG, JPEG, WebP, and GIF images, as a local file or a URL. The deployment must use a model that accepts images.
package main

import (
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/azureopenai"
)

func main() {
	file, question := contentdemo.Args("azureopenai")
	llm := azureopenai.NewClient(
		contentdemo.Env("AZURE_OPENAI_API_KEY"),
		contentdemo.Env("AZURE_OPENAI_BASE_URL"),
		contentdemo.Env("AZURE_OPENAI_DEPLOYMENT"),
	)

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
