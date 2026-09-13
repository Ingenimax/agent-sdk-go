package docsample

import (
	"context"
	"strings"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/internal/testutil"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/guardrails"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/memory"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// --- sample: building a pipeline and attaching it -------------------------

func TestDocSamplePipeline(t *testing.T) {
	gr := guardrails.NewPipeline([]guardrails.Guardrail{
		guardrails.NewPiiFilter(guardrails.RedactAction),
		guardrails.NewContentFilter([]string{"badword"}, guardrails.BlockAction),
		guardrails.NewTokenLimit(4000, nil, guardrails.RedactAction, "end"),
	}, logging.New())

	if _, err := agent.NewAgent(
		agent.WithLLM(testutil.NewFakeLLM()),
		agent.WithMemory(memory.NewConversationBuffer()),
		agent.WithGuardrails(gr),
	); err != nil {
		t.Fatalf("the documented wiring does not work: %v", err)
	}
}

// --- sample: custom guardrail ---------------------------------------------

type MyGuardrail struct{}

func (g *MyGuardrail) Type() guardrails.GuardrailType { return "my_guardrail" }
func (g *MyGuardrail) Action() guardrails.Action      { return guardrails.RedactAction }

func (g *MyGuardrail) CheckRequest(ctx context.Context, request string) (bool, string, error) {
	if strings.Contains(request, "forbidden") {
		return true, strings.ReplaceAll(request, "forbidden", "[REDACTED]"), nil
	}
	return false, request, nil
}

func (g *MyGuardrail) CheckResponse(ctx context.Context, response string) (bool, string, error) {
	return false, response, nil
}

func TestDocSampleCustomGuardrail(t *testing.T) {
	p := guardrails.NewPipeline([]guardrails.Guardrail{&MyGuardrail{}}, logging.New())
	got, err := p.ProcessInput(context.Background(), "this is forbidden")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if strings.Contains(got, "forbidden") {
		t.Errorf("got %q, want redacted", got)
	}
}

// --- sample: per-org pipelines --------------------------------------------

type PerOrg struct {
	byOrg    map[string]*guardrails.Pipeline
	fallback *guardrails.Pipeline
}

func (p *PerOrg) pipeline(ctx context.Context) *guardrails.Pipeline {
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return p.fallback
	}
	if gr, ok := p.byOrg[orgID]; ok {
		return gr
	}
	return p.fallback
}

func (p *PerOrg) ProcessInput(ctx context.Context, input string) (string, error) {
	return p.pipeline(ctx).ProcessInput(ctx, input)
}

func (p *PerOrg) ProcessOutput(ctx context.Context, output string) (string, error) {
	return p.pipeline(ctx).ProcessOutput(ctx, output)
}

func TestDocSamplePerOrgSatisfiesTheInterface(t *testing.T) {
	var _ interfaces.Guardrails = (*PerOrg)(nil)

	p := &PerOrg{
		byOrg: map[string]*guardrails.Pipeline{
			"org-1": guardrails.NewPipeline([]guardrails.Guardrail{
				guardrails.NewContentFilter([]string{"alpha"}, guardrails.RedactAction),
			}, logging.New()),
		},
		fallback: guardrails.NewPipeline(nil, logging.New()),
	}

	got, err := p.ProcessInput(multitenancy.WithOrgID(context.Background(), "org-1"), "alpha here")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if strings.Contains(got, "alpha") {
		t.Errorf("got %q, want the org-1 policy applied", got)
	}

	// An unknown org falls back and is left alone.
	got, err = p.ProcessInput(multitenancy.WithOrgID(context.Background(), "org-9"), "alpha here")
	if err != nil {
		t.Fatalf("ProcessInput() error = %v", err)
	}
	if got != "alpha here" {
		t.Errorf("got %q, want the fallback to leave it alone", got)
	}
}

// --- sample: middleware ----------------------------------------------------

func TestDocSampleToolMiddleware(t *testing.T) {
	gr := guardrails.NewPipeline(nil, logging.New())
	guarded := guardrails.NewToolMiddleware(&testutil.FakeTool{ToolName: "t"}, gr)
	if guarded.Name() != "t" {
		t.Errorf("Name() = %q", guarded.Name())
	}
}
