// Package contentdemo holds the code the content examples share. It sends a
// file and a question to an agent with RunWithContent, asks a follow-up
// question with Run, which the agent answers from memory, and then sends the
// same file to the client directly with WithParts.
package contentdemo

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Args returns the file and the question from the command line, or exits
// with a usage message naming the example.
func Args(example string) (file, question string) {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: go run ./examples/content/%s <file path or URL> <question>\n", example)
		os.Exit(2)
	}
	return os.Args[1], os.Args[2]
}

// Env returns the value of an environment variable, or exits when it is not
// set.
func Env(name string) string {
	value := os.Getenv(name)
	if value == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", name)
		os.Exit(2)
	}
	return value
}

// Run sends the file and the question to an agent backed by llm, prints the
// answer, asks a follow-up question about the same file, and then sends the
// file and the question to llm directly.
func Run(llm interfaces.LLM, file, question string) error {
	part, err := Part(file)
	if err != nil {
		return err
	}

	a, err := agent.NewAgent(
		agent.WithLLM(llm),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithSystemPrompt("You answer questions about the files the user sends."),
	)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	ctx := multitenancy.WithOrgID(context.Background(), "example-org")
	ctx = memory.WithConversationID(ctx, "content-example")

	content := interfaces.NewContentFromParts([]*interfaces.Part{
		part,
		interfaces.NewPartFromText(question),
	}, interfaces.MessageRoleUser)

	answer, err := a.RunWithContent(ctx, content)
	if err != nil {
		return fmt.Errorf("run with content: %w", err)
	}
	fmt.Printf("Answer:\n%s\n\n", answer)

	// Memory stores the file with the first message, so the follow-up
	// question can refer to it without sending it again.
	followUp, err := a.Run(ctx, "In one sentence, what kind of file did I send you?")
	if err != nil {
		return fmt.Errorf("run follow-up: %w", err)
	}
	fmt.Printf("Follow-up:\n%s\n\n", followUp)

	// Without an agent, call the client with WithParts. The parts replace
	// the prompt, so the question goes in as a text part.
	direct, err := llm.Generate(ctx, "", interfaces.WithParts(part, interfaces.NewPartFromText(question)))
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	fmt.Printf("Direct client call:\n%s\n", direct)
	return nil
}

// Part returns a part for file. A URL (http, https, gs, or s3) becomes file
// data, with the MIME type taken from its extension. A local path becomes
// inline data, with the MIME type detected from its content. Either way the
// file name is set as the display name, which some providers send.
func Part(file string) (*interfaces.Part, error) {
	name := filepath.Base(file)
	for _, scheme := range []string{"http://", "https://", "gs://", "s3://"} {
		if !strings.HasPrefix(file, scheme) {
			continue
		}
		mimeType := mime.TypeByExtension(filepath.Ext(file))
		if mimeType == "" {
			return nil, fmt.Errorf("cannot tell the MIME type of %s from its extension", file)
		}
		part := interfaces.NewPartFromURI(file, baseType(mimeType))
		part.FileData.DisplayName = name
		return part, nil
	}

	data, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return nil, err
	}
	part := interfaces.NewPartFromBytes(data, baseType(http.DetectContentType(data)))
	part.InlineData.DisplayName = name
	return part, nil
}

// baseType drops parameters such as "; charset=utf-8" from a MIME type.
func baseType(mimeType string) string {
	base, _, _ := strings.Cut(mimeType, ";")
	return strings.TrimSpace(base)
}
