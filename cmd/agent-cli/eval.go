package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	agenteval "github.com/Ingenimax/agent-sdk-go/pkg/eval"
	evalsdk "github.com/Ingenimax/agent-sdk-go/pkg/eval/sdk"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/anthropic"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/gemini"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/ollama"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/vllm"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"google.golang.org/genai"
)

const (
	evalExitPass      = 0
	evalExitFail      = 1
	evalExitError     = 2
	evalExitInterrupt = 130
)

func init() {
	evalCommand = runEvalCommand
}

type evalCommandOptions struct {
	datasetPath         string
	configPath          string
	observationsPath    string
	format              string
	outputPath          string
	includeObservations bool
	concurrency         int
	caseTimeout         time.Duration
	gradingTimeout      time.Duration
	cleanupTimeout      time.Duration
	maxSpans            int
	maxContentBytes     int
	judgeProvider       string
	judgeModel          string
}

func runEvalCommand(args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	options, err := parseEvalFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return evalExitPass
		}
		return evalExitError
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	evaluators, err := createEvalEvaluators(ctx, options)
	if err != nil {
		evalDiagnostic(stderr, "evaluation: %v\n", err)
		return evalExitError
	}

	datasetFile, err := os.Open(options.datasetPath) // #nosec G304 -- explicitly supplied CLI input
	if err != nil {
		evalDiagnostic(stderr, "evaluation: open dataset: %v\n", err)
		return evalExitError
	}
	dataset, loadErr := agenteval.LoadDatasetWithEvaluators(datasetFile, evaluators)
	closeErr := datasetFile.Close()
	if loadErr != nil {
		evalDiagnostic(stderr, "evaluation: %v\n", loadErr)
		return evalExitError
	}
	if closeErr != nil {
		evalDiagnostic(stderr, "evaluation: close dataset: %v\n", closeErr)
		return evalExitError
	}

	var report agenteval.Report
	var runErr error
	if options.observationsPath != "" {
		report, runErr = regradeFromFile(ctx, dataset, options.observationsPath, evaluators)
	} else {
		report, runErr = runLiveEvaluation(ctx, dataset, options, evaluators)
	}
	if report.SchemaVersion != "" {
		if err := writeEvaluationReport(stdout, options, report); err != nil {
			evalDiagnostic(stderr, "evaluation: %v\n", err)
			return evalExitError
		}
	}
	if runErr != nil {
		evalDiagnostic(stderr, "evaluation: %v\n", runErr)
		if errors.Is(runErr, context.Canceled) {
			return evalExitInterrupt
		}
		return evalExitError
	}

	evalDiagnostic(stderr, "evaluation: %d passed, %d failed, %d errors, %d not started\n",
		report.Summary.Passed,
		report.Summary.Failed,
		report.Summary.Errors,
		report.Summary.NotStarted,
	)
	if report.Summary.Errors > 0 || report.Summary.NotStarted > 0 {
		return evalExitError
	}
	if report.Summary.Failed > 0 {
		return evalExitFail
	}
	return evalExitPass
}

func parseEvalFlags(args []string, stderr io.Writer) (evalCommandOptions, error) {
	var options evalCommandOptions
	flags := flag.NewFlagSet("eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.datasetPath, "dataset", "", "path to the evaluation dataset JSON")
	flags.StringVar(&options.configPath, "config", "", "path to an agent-cli JSON configuration")
	flags.StringVar(&options.observationsPath, "observations", "", "full JSON report to regrade without running an agent")
	flags.StringVar(&options.format, "format", "json", "report format: json or junit")
	flags.StringVar(&options.outputPath, "output", "-", "report output path, or - for stdout")
	flags.BoolVar(&options.includeObservations, "include-observations", false, "include full observations needed for later regrading")
	flags.IntVar(&options.concurrency, "concurrency", 1, "maximum cases to execute concurrently")
	flags.DurationVar(&options.caseTimeout, "timeout", 0, "deadline for setup and execution of each case")
	flags.DurationVar(&options.gradingTimeout, "grading-timeout", 30*time.Second, "deadline for grading each case")
	flags.DurationVar(&options.cleanupTimeout, "cleanup-timeout", 10*time.Second, "deadline for cleanup of each case")
	flags.IntVar(&options.maxSpans, "max-spans", agenteval.DefaultMaxSpans, "maximum retained tool spans per case")
	flags.IntVar(&options.maxContentBytes, "max-content-bytes", agenteval.DefaultMaxContentBytes, "maximum retained output, trace, and export content bytes")
	flags.StringVar(&options.judgeProvider, "judge-provider", "", "provider for model_judge checks")
	flags.StringVar(&options.judgeModel, "judge-model", "", "model for model_judge checks")
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		err := fmt.Errorf("unexpected arguments: %v", flags.Args())
		evalDiagnostic(stderr, "evaluation: %v\n", err)
		return options, err
	}
	if options.datasetPath == "" {
		evalDiagnostic(stderr, "evaluation: --dataset is required\n")
		return options, errors.New("dataset is required")
	}
	if options.format != "json" && options.format != "junit" {
		evalDiagnostic(stderr, "evaluation: --format must be json or junit\n")
		return options, errors.New("invalid report format")
	}
	if options.concurrency <= 0 || options.maxSpans <= 0 || options.maxContentBytes <= 0 {
		evalDiagnostic(stderr, "evaluation: concurrency and retention limits must be positive\n")
		return options, errors.New("invalid numeric option")
	}
	if options.caseTimeout < 0 || options.gradingTimeout < 0 || options.cleanupTimeout < 0 {
		evalDiagnostic(stderr, "evaluation: durations cannot be negative\n")
		return options, errors.New("invalid duration")
	}
	if (options.judgeProvider == "") != (options.judgeModel == "") {
		evalDiagnostic(stderr, "evaluation: --judge-provider and --judge-model must be set together\n")
		return options, errors.New("incomplete model judge configuration")
	}
	return options, nil
}

func evalDiagnostic(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, format, args...)
}

func runLiveEvaluation(ctx context.Context, dataset agenteval.Dataset, options evalCommandOptions, evaluators agenteval.Evaluators) (agenteval.Report, error) {
	config, configBytes, err := loadEvalConfig(options.configPath)
	if err != nil {
		return agenteval.Report{}, err
	}
	fingerprintBytes := configBytes
	if options.judgeProvider != "" {
		fingerprintBytes, err = json.Marshal(struct {
			Agent         json.RawMessage `json:"agent"`
			JudgeProvider string          `json:"judge_provider"`
			JudgeModel    string          `json:"judge_model"`
		}{
			Agent:         configBytes,
			JudgeProvider: options.judgeProvider,
			JudgeModel:    options.judgeModel,
		})
		if err != nil {
			return agenteval.Report{}, fmt.Errorf("encode evaluation fingerprint: %w", err)
		}
	}
	digest := sha256.Sum256(fingerprintBytes)
	runner := agenteval.Runner{
		Evaluators: evaluators,
		Options: agenteval.RunnerOptions{
			Concurrency:       options.concurrency,
			CaseTimeout:       options.caseTimeout,
			GradingTimeout:    options.gradingTimeout,
			CleanupTimeout:    options.cleanupTimeout,
			ConfigFingerprint: "sha256:" + hex.EncodeToString(digest[:]),
			BuildRevision:     evaluationBuildRevision(),
			Recorder: agenteval.RecorderOptions{
				MaxSpans:        options.maxSpans,
				MaxContentBytes: options.maxContentBytes,
			},
		},
		Factory: func(setupContext context.Context, evalCase agenteval.Case, recorder *agenteval.Recorder) (agenteval.Target, error) {
			subject, err := createEvalAgent(setupContext, config, recorder)
			if err != nil {
				return agenteval.Target{}, err
			}
			return agenteval.Target{
				Subject: cliEvalSubject{
					agent:          subject,
					conversationID: config.ConversationID + ":eval:" + evalCase.ID,
				},
				Capabilities: agenteval.Capabilities{
					ToolCapture: agenteval.CoverageComplete,
					Usage:       agenteval.CoverageComplete,
					Output:      agenteval.CoverageComplete,
				},
			}, nil
		},
	}
	return runner.Run(ctx, dataset)
}

func evaluationBuildRevision() string {
	information, ok := debug.ReadBuildInfo()
	if !ok {
		return "agent-cli-" + version
	}
	revision := ""
	dirty := false
	for _, setting := range information.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision != "" {
		if dirty {
			return revision + "+dirty"
		}
		return revision
	}
	if information.Main.Version != "" && information.Main.Version != "(devel)" {
		return information.Main.Version
	}
	return "agent-cli-" + version
}

func regradeFromFile(ctx context.Context, dataset agenteval.Dataset, path string, evaluators agenteval.Evaluators) (agenteval.Report, error) {
	file, err := os.Open(path) // #nosec G304 -- explicitly supplied CLI input
	if err != nil {
		return agenteval.Report{}, fmt.Errorf("open observations: %w", err)
	}
	previous, readErr := agenteval.ReadReport(file)
	closeErr := file.Close()
	if readErr != nil {
		return agenteval.Report{}, readErr
	}
	if closeErr != nil {
		return agenteval.Report{}, fmt.Errorf("close observations: %w", closeErr)
	}
	return agenteval.Regrade(ctx, dataset, previous, evaluators)
}

func writeEvaluationReport(stdout io.Writer, options evalCommandOptions, report agenteval.Report) error {
	if options.outputPath == "-" {
		if options.format == "junit" {
			return agenteval.WriteJUnit(stdout, report)
		}
		return agenteval.WriteJSON(stdout, report, agenteval.ExportOptions{
			IncludeObservations: options.includeObservations,
			MaxContentBytes:     options.maxContentBytes,
		})
	}
	if options.format == "junit" {
		return agenteval.WriteJUnitFile(options.outputPath, report)
	}
	return agenteval.WriteJSONFile(options.outputPath, report, agenteval.ExportOptions{
		IncludeObservations: options.includeObservations,
		MaxContentBytes:     options.maxContentBytes,
	})
}

func loadEvalConfig(path string) (*CLIConfig, []byte, error) {
	explicitPath := path != ""
	if path == "" {
		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(homeDirectory, ".agent-cli", "config.json")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- config path is explicit or under the user's home directory
	if errors.Is(err, os.ErrNotExist) {
		if explicitPath {
			return nil, nil, fmt.Errorf("read config: %w", err)
		}
		provider := os.Getenv("LLM_PROVIDER")
		if provider == "" {
			provider = "openai"
		}
		config := &CLIConfig{
			Provider:       provider,
			Model:          getDefaultModel(provider),
			SystemPrompt:   "You are a helpful AI assistant.",
			Temperature:    0.7,
			MaxIterations:  2,
			OrgID:          "default-org",
			ConversationID: "cli-session",
			EnableMemory:   true,
			EnableTools:    true,
			Variables:      make(map[string]string),
		}
		canonical, marshalErr := json.Marshal(config)
		return config, canonical, marshalErr
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read config: %w", err)
	}
	var config CLIConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	if provider := os.Getenv("LLM_PROVIDER"); provider != "" {
		config.Provider = provider
		if config.Model == "" {
			config.Model = getDefaultModel(provider)
		}
	}
	if config.Model == "" {
		config.Model = getDefaultModel(config.Provider)
	}
	canonical, err := json.Marshal(&config)
	if err != nil {
		return nil, nil, fmt.Errorf("canonicalize config: %w", err)
	}
	return &config, canonical, nil
}

func createEvalAgent(ctx context.Context, config *CLIConfig, recorder *agenteval.Recorder) (*agent.Agent, error) {
	llm, err := createEvalLLM(ctx, config)
	if err != nil {
		return nil, err
	}
	options := []agent.Option{
		agent.WithLLM(llm),
		agent.WithSystemPrompt(config.SystemPrompt),
		agent.WithLLMConfig(interfaces.LLMConfig{Temperature: config.Temperature}),
		agent.WithMaxIterations(config.MaxIterations),
		agent.WithName("CLI-Agent"),
		agent.WithOrgID(config.OrgID),
		agent.WithRequirePlanApproval(false),
	}
	if config.EnableMemory {
		options = append(options, agent.WithMemory(memory.NewConversationBuffer()))
	}
	if config.EnableTools {
		if toolSet := createTools(); len(toolSet) > 0 {
			options = append(options, agent.WithTools(toolSet...))
		}
	}
	if len(config.MCPServers) > 0 {
		if lazyConfigs := createLazyMCPConfigs(config.MCPServers, nil); len(lazyConfigs) > 0 {
			options = append(options, agent.WithLazyMCPConfigs(lazyConfigs))
		}
	}
	if config.EnableTracing {
		if tracer := createTracer(); tracer != nil {
			options = append(options, agent.WithTracer(tracer))
		}
	}
	options = append(options, evalsdk.Options(recorder, agenteval.RootAgentPath)...)
	return agent.NewAgent(options...)
}

func createEvalEvaluators(ctx context.Context, options evalCommandOptions) (agenteval.Evaluators, error) {
	evaluators := agenteval.BuiltinEvaluators()
	if options.judgeProvider == "" {
		return evaluators, nil
	}
	model, err := createEvalJudgeLLM(ctx, options.judgeProvider, options.judgeModel)
	if err != nil {
		return nil, fmt.Errorf("configure model judge: %w", err)
	}
	evaluators[agenteval.EvaluatorModelJudge] = agenteval.NewModelJudge(model)
	return evaluators, nil
}

func createEvalJudgeLLM(ctx context.Context, provider, model string) (interfaces.LLM, error) { //nolint:staticcheck
	return createEvalLLMWithOverrides(ctx, &CLIConfig{Provider: provider, Model: model},
		"AGENT_EVAL_JUDGE_API_KEY",
		"AGENT_EVAL_JUDGE_BASE_URL",
		"AGENT_EVAL_JUDGE_PROJECT_ID",
	)
}

func createEvalLLM(ctx context.Context, config *CLIConfig) (interfaces.LLM, error) { //nolint:staticcheck
	return createEvalLLMWithOverrides(ctx, config, "", "", "")
}

func createEvalLLMWithOverrides(
	ctx context.Context,
	config *CLIConfig,
	apiKeyOverride string,
	baseURLOverride string,
	projectIDOverride string,
) (interfaces.LLM, error) { //nolint:staticcheck
	if config == nil {
		return nil, errors.New("agent configuration is nil")
	}
	switch config.Provider {
	case "openai":
		apiKey := evalEnvironmentValue(apiKeyOverride, "OPENAI_API_KEY")
		if apiKey == "" {
			return nil, errors.New("OPENAI_API_KEY is required for the OpenAI provider")
		}
		options := []openai.Option{openai.WithModel(config.Model)}
		if baseURL := evalEnvironmentValue(baseURLOverride, "OPENAI_BASE_URL"); baseURL != "" {
			options = append(options, openai.WithBaseURL(baseURL))
		}
		return openai.NewClient(apiKey, options...), nil
	case "anthropic":
		apiKey := evalEnvironmentValue(apiKeyOverride, "ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, errors.New("ANTHROPIC_API_KEY is required for the Anthropic provider")
		}
		options := []anthropic.Option{anthropic.WithModel(config.Model)}
		if baseURL := evalEnvironmentValue(baseURLOverride, "ANTHROPIC_BASE_URL"); baseURL != "" {
			options = append(options, anthropic.WithBaseURL(baseURL))
		}
		return anthropic.NewClient(apiKey, options...), nil
	case "vertex":
		projectID := evalEnvironmentValue(projectIDOverride, "GOOGLE_CLOUD_PROJECT")
		if projectID == "" {
			return nil, errors.New("GOOGLE_CLOUD_PROJECT is required for the Vertex AI provider")
		}
		client, err := gemini.NewClient(ctx,
			gemini.WithBackend(genai.BackendVertexAI),
			gemini.WithProjectID(projectID),
			gemini.WithModel(config.Model),
		)
		if err != nil {
			return nil, fmt.Errorf("create Vertex AI client: %w", err)
		}
		return client, nil
	case "ollama":
		baseURL := evalEnvironmentValue(baseURLOverride, "OLLAMA_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return ollama.NewClient(ollama.WithBaseURL(baseURL), ollama.WithModel(config.Model)), nil
	case "vllm":
		baseURL := evalEnvironmentValue(baseURLOverride, "VLLM_BASE_URL")
		if baseURL == "" {
			baseURL = "http://localhost:8000"
		}
		return vllm.NewClient(vllm.WithBaseURL(baseURL), vllm.WithModel(config.Model)), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", config.Provider)
	}
}

func evalEnvironmentValue(override, fallback string) string {
	if override != "" {
		if value := os.Getenv(override); value != "" {
			return value
		}
	}
	return os.Getenv(fallback)
}

type cliEvalSubject struct {
	agent          *agent.Agent
	conversationID string
}

func (s cliEvalSubject) RunDetailed(ctx context.Context, input string) (*interfaces.AgentResponse, error) {
	ctx = memory.WithConversationID(ctx, s.conversationID)
	return s.agent.RunDetailed(ctx, input)
}
