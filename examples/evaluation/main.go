package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/eval"
	evalsdk "github.com/Ingenimax/agent-sdk-go/pkg/eval/sdk"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func main() {
	datasetPath := flag.String("dataset", "examples/evaluation/dataset.json", "evaluation dataset")
	flag.Parse()

	file, err := os.Open(*datasetPath) // #nosec G304 -- example CLI input
	if err != nil {
		log.Fatal(err)
	}
	dataset, err := eval.LoadDataset(file)
	closeErr := file.Close()
	if err != nil {
		log.Fatal(err)
	}
	if closeErr != nil {
		log.Fatal(closeErr)
	}

	runner := eval.Runner{
		Factory: func(_ context.Context, _ eval.Case, recorder *eval.Recorder) (eval.Target, error) {
			options := []agent.Option{
				agent.WithName("weather-agent"),
				agent.WithSystemPrompt("Answer weather questions with the available fixture tool."),
				agent.WithLogger(discardLogger{}),
				agent.WithLLM(weatherLLM{}),
				agent.WithTools(weatherTool{}),
				agent.WithRequirePlanApproval(false),
				agent.WithDisableFinalSummary(true),
			}
			options = append(options, evalsdk.Options(recorder, eval.RootAgentPath)...)
			subject, err := agent.NewAgent(options...)
			if err != nil {
				return eval.Target{}, err
			}
			return evalsdk.NewTarget(subject, nil), nil
		},
	}
	report, runErr := runner.Run(context.Background(), dataset)
	if err := eval.WriteJSON(os.Stdout, report, eval.ExportOptions{IncludeObservations: true}); err != nil {
		log.Fatal(err)
	}
	if runErr != nil {
		log.Fatal(runErr)
	}
	if report.Summary.Passed != report.Summary.Total {
		os.Exit(1)
	}
}

type discardLogger struct{}

func (discardLogger) Info(context.Context, string, map[string]interface{})  {}
func (discardLogger) Warn(context.Context, string, map[string]interface{})  {}
func (discardLogger) Error(context.Context, string, map[string]interface{}) {}
func (discardLogger) Debug(context.Context, string, map[string]interface{}) {}

type weatherTool struct{}

func (weatherTool) Name() string        { return "weather" }
func (weatherTool) Description() string { return "Return fixture weather data" }
func (weatherTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"city": {Type: "string", Required: true},
	}
}
func (weatherTool) Run(_ context.Context, city string) (string, error) {
	return fmt.Sprintf("%s: 22°C", city), nil
}
func (weatherTool) Execute(_ context.Context, arguments string) (string, error) {
	if arguments != `{"city":"Madrid"}` {
		return "", fmt.Errorf("unexpected fixture arguments %s", arguments)
	}
	return "Madrid: 22°C", nil
}

type weatherLLM struct{}

func (weatherLLM) Name() string            { return "fixture" }
func (weatherLLM) SupportsStreaming() bool { return false }
func (weatherLLM) Generate(context.Context, string, ...interfaces.GenerateOption) (string, error) {
	return "Madrid is 22°C", nil
}
func (weatherLLM) GenerateWithTools(ctx context.Context, _ string, tools []interfaces.Tool, _ ...interfaces.GenerateOption) (string, error) {
	if len(tools) != 1 {
		return "", fmt.Errorf("got %d tools, want 1", len(tools))
	}
	if _, err := tools[0].Execute(ctx, `{"city":"Madrid"}`); err != nil {
		return "", err
	}
	return "Madrid is 22°C", nil
}
func (llm weatherLLM) GenerateDetailed(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	content, err := llm.Generate(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}
	return fixtureResponse(content), nil
}
func (llm weatherLLM) GenerateWithToolsDetailed(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (*interfaces.LLMResponse, error) {
	content, err := llm.GenerateWithTools(ctx, prompt, tools, options...)
	if err != nil {
		return nil, err
	}
	return fixtureResponse(content), nil
}

func fixtureResponse(content string) *interfaces.LLMResponse {
	return &interfaces.LLMResponse{
		Content: content,
		Model:   "fixture-model",
		Usage: &interfaces.TokenUsage{
			InputTokens:  5,
			OutputTokens: 3,
			TotalTokens:  8,
		},
	}
}
