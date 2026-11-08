// Command bedrock sends a file and a question to an agent backed by Amazon Bedrock (Converse).
//
//	BEDROCK_API_KEY=... BEDROCK_MODEL=us.amazon.nova-pro-v1:0 go run ./examples/content/bedrock report.pdf "Summarize this report."
//
// It takes images, documents (PDF, Word, Excel, CSV, HTML, text, and Markdown), video, and audio, as a
// local file or an s3:// URI. Which kinds work depends on the model, and only
// Amazon Nova models take s3:// URIs. Bedrock needs a name for each document;
// the example uses the file name.
package main

import (
	"context"
	"log"

	"github.com/Ingenimax/agent-sdk-go/examples/content/contentdemo"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/bedrock"
)

func main() {
	file, question := contentdemo.Args("bedrock")
	llm, err := bedrock.NewClient(context.Background(),
		contentdemo.Env("BEDROCK_API_KEY"),
		bedrock.WithModel(contentdemo.Env("BEDROCK_MODEL")),
	)
	if err != nil {
		log.Fatal(err)
	}

	if err := contentdemo.Run(llm, file, question); err != nil {
		log.Fatal(err)
	}
}
