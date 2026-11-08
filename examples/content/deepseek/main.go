// Command deepseek sends a file and a question to an agent backed by DeepSeek.
//
//	DEEPSEEK_API_KEY=... go run ./examples/content/deepseek chart.png "What trend does this chart show?"
//
// It takes JPEG, PNG, GIF, and WebP images, as a local file or a URL. Only the deepseek-flash model
// accepts images.
package main

import (
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/deepseek"
)

func main() {
	file, question := contentdemo.Args("deepseek")
	llm := deepseek.NewClient(contentdemo.Env("DEEPSEEK_API_KEY"), deepseek.WithModel("deepseek-flash"))

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
