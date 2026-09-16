# Evaluation framework: technical implementation plan

Status: implemented in `pkg/eval`, `pkg/eval/sdk`, and the `agent-cli eval`
command, including the optional model-judge phase.

## Objective and first release

Add a provider-neutral evaluation library that runs an agent against a versioned
dataset, captures its observable behavior, and produces reproducible grading
results. Users must be able to detect regressions in answers, tool selection,
arguments, and resource use without a hosted evaluation service.

The first release supports single-input cases, local agents through
`RunDetailed`, isolated execution, deterministic graders, offline regrading of
saved traces, JSON/JUnit reports, CLI execution, and opt-in model-based judges.

Deterministic grading means identical results for identical observations and
configuration. Live agent execution remains nondeterministic, even at zero
temperature. Latency is an observed measurement, not a reproducibility guarantee.

Defer multi-turn conversations, streaming capture, durable execution, automatic
dataset generation, dashboards, model optimization, and monetary cost estimation.
Do not change `interfaces.LLM` or individual provider loops for the first release.

## Existing integration points and constraints

| Existing code | Use and constraint |
| --- | --- |
| [agent.go](../pkg/agent/agent.go), `Agent.RunDetailed` | Provides final content, optional usage, and execution summary. Failed runs can return a nil response; preserve the independent trace. |
| [tool_pipeline.go](../pkg/agent/tool_pipeline.go), `WithToolDecorator` | Wraps tools before provider dispatch. Covers both `Run` and `Execute` without provider changes. Later decorators are outermost. |
| [hooks.go](../pkg/hooks/hooks.go) | Policy hooks can modify or deny calls. Current callbacks lack invocation IDs and an unconditional completion callback, so they cannot alone produce a reliably correlated trace. |
| [interfaces/agent.go](../pkg/interfaces/agent.go) | Reuse response and usage types in memory; define explicit, versioned report DTOs instead of serializing arbitrary metadata. |
| [interfaces/streaming.go](../pkg/interfaces/streaming.go) | Future streaming adapter can consume completion metadata. Tool events must not be counted again alongside decorator events. |
| [cmd/agent-cli/main.go](../cmd/agent-cli/main.go) | Existing command switch and construction helpers need an additive `eval` command and error-returning construction path. No CLI framework migration is needed. |
| [internal/testutil/fakes.go](../internal/testutil/fakes.go) | Reuse or extend fake models/tools for offline integration tests. |

Usage summaries do not expose every provider-internal model request. Do not label
`ExecutionSummary.LLMCalls` as an exact HTTP request count. Session or run-manager
persistence is not a prerequisite for evaluating one completed run.

## Package boundaries

```text
pkg/eval/
  types.go           Dataset, Case, Observation, Trace, results and interfaces
  dataset.go         strict loading, validation and canonical dataset digest
  runner.go          execution, isolation, cancellation and aggregation
  recorder.go        correlated tool spans and bounded trace snapshots
  grade.go           offline grading of observations
  graders.go         built-in deterministic evaluators and config validation
  model_judge.go     optional injected-model rubric evaluator
  report.go          versioned JSON and JUnit export/import
  sdk/
    target.go        adapter for agent.Agent and its tool decorators
  testdata/          datasets, traces and report fixtures
cmd/agent-cli/
  eval.go            command parsing and library integration
examples/evaluation/
  main.go            runnable example with fake tools
  dataset.json
docs/evaluation.md   user guide for the implemented API
```

Dependency direction: `eval/sdk -> eval + agent + interfaces`; core `eval` may
import `interfaces`, but must not import `agent` or a provider. The model judge
accepts an injected model interface. Existing agent/provider packages never
import `eval`. Keep concrete built-in graders in `eval` initially; avoid a plugin
registry or dynamic loading beyond an explicit evaluator map supplied by the
caller.

Reuse the repository's existing YAML and JSON Schema dependencies after checking
their supported schema dialect during implementation. Do not introduce a second
schema engine. The first PR may accept JSON only; YAML support must use identical
validation semantics before being advertised.

## Proposed Go contracts

The following signatures define the intended boundary; supporting structs and
serialization tags are specified below and will be finalized in the first PR.

```go
type Subject interface {
    RunDetailed(context.Context, string) (*interfaces.AgentResponse, error)
}

type Target struct {
    Subject Subject
    Close   func(context.Context) error
}

// Called once per case attempt. Builds fresh mutable state and installs tracing.
type TargetFactory func(context.Context, Case, *Recorder) (Target, error)

type Evaluator interface {
    Name() string
    Validate(Check) error
    Evaluate(context.Context, Evaluation) (MetricResult, error)
}

type Runner struct {
    Factory    TargetFactory
    Evaluators map[string]Evaluator
    Options    RunnerOptions
}

func (r *Runner) Run(context.Context, Dataset) (Report, error)
func Grade(context.Context, Case, Observation,
    map[string]Evaluator) (CaseResult, error)
```

An evaluator returns a valid metric result for an observed mismatch. Its Go
`error` is reserved for evaluation failure, such as malformed judge output or an
internal error; the runner records that as an errored metric, never as score zero.
Case-level execution errors belong in the report. `Runner.Run` returns an error
for invalid global configuration, suite cancellation, or failures that prevent
normal suite execution, preserving any partial report.

The SDK adapter should provide `sdk.Decorator(recorder, agentPath, layer)` and a
factory helper that assembles fresh agents with the ordering described below.
Callers with custom construction can install the decorators themselves. Explicitly
declare observation capabilities; an uninstrumented custom `Subject` must not
appear to have made zero tool calls.

## Dataset and validation contract

Use `schema_version: "1"` with unique dataset, case, and check IDs. A case contains
`input`, optional `reference`, optional string tags, and a nonempty `checks` list.
The reference is inert unless a configured evaluator consumes it. Every check
contains `id`, `type`, type-specific `config`, `threshold`, and `required`.
Threshold defaults to `1`; required defaults to `true`.

```json
{
  "schema_version": "1",
  "id": "weather-agent",
  "cases": [
    {
      "id": "madrid-weather",
      "input": "What is the temperature in Madrid?",
      "checks": [
        {
          "id": "answer",
          "type": "output_contains",
          "config": {"value": "22", "normalization": "none"}
        },
        {
          "id": "tools",
          "type": "tool_trajectory",
          "config": {
            "layer": "execution",
            "mode": "exact",
            "calls": [
              {"name": "weather", "arguments": {"city": "Madrid"}}
            ]
          }
        },
        {
          "id": "call-limit",
          "type": "max_tool_calls",
          "config": {"layer": "execution", "maximum": 1}
        }
      ]
    }
  ]
}
```

The example requires a fixture weather tool returning 22 degrees. Tool fixtures,
credentials, and agent construction belong to the factory, not the dataset.

Validate the whole suite before constructing agents: reject unknown versions,
unknown fields/types, duplicate IDs or object keys, empty check lists, invalid
regex/schema definitions, nonfinite/out-of-range thresholds, negative limits,
and invalid UTF-8/oversized inputs. JSON numbers must retain precision through
`json.Number`; do not round arguments through `float64`. Accept exactly one JSON
document. YAML, when added, must reject duplicate keys, unknown fields, and
unbounded alias expansion; use JSON-compatible scalar semantics.

Use explicit defaults and emit the resolved configuration in reports. Compute a
SHA-256 digest from a documented canonical JSON encoding of the validated
dataset, preserving case/check array order. Restrict schemas to bundled/local
definitions with no automatic network `$ref` resolution.

## Observation and trace model

`Observation` contains final output (nullable), run status, structured error,
monotonic elapsed duration, optional token usage, execution summary, and `Trace`.
Store capabilities for output, tool capture, and usage coverage (`complete`,
`partial`, `unavailable`). Complete tool capture means all tools in the evaluated
scope are instrumented, not that remote agent internals are visible.

Each `ToolSpan` contains an evaluation-local ID, parent span ID, agent path,
layer (`attempt` or `execution`), start sequence number, tool name, entry method
(`Run`/`Execute`), raw arguments, result, error, start/end times, and completion
state. IDs need not match provider tool-call IDs. Use an atomic counter for IDs
and start order and a mutex for snapshots. Never hold a recorder lock while
calling tools. Copy mutable values before publishing an observation to graders.

Use a dedicated decorator rather than reconstructing spans from hook callback
order. It allocates an ID before invocation, carries parent identity through a
private context key, and closes that exact span on return. Record panics in a
deferred function and re-panic; the evaluator must not silently change application
panic semantics. Forward `Unwrap`, display name, and internal-tool interfaces.

### Decorator ordering and policy semantics

For the supplied SDK factory, construct this pipeline:

```text
provider -> attempt recorder -> policy hooks -> execution recorder
         -> existing usage tracker -> actual tool
```

Register the execution recorder first, policy decorators next, and attempt
recorder last. An attempt records provider-supplied arguments and the result
visible to the provider. Its child execution span records modified arguments and
actual execution result/error. A denied or otherwise short-circuited attempt has
no execution child; report `short_circuited`, not a fabricated denial reason.
Recovered execution errors remain visible in the execution span even when the
outer attempt succeeds. Graders specify a layer and never sum both layers.

Instrument child agents explicitly with the same recorder and unique agent paths;
context parent IDs connect nested work. Default trajectory scope is the root
agent's tools. Remote/custom agents with inaccessible internals advertise partial
coverage. A grader requiring that missing scope produces `unavailable`.

Capture order is invocation start order, not completion order. Exact trajectory
checks are appropriate for sequential behavior. For concurrent calls, support
unordered matching with multiplicity; do not assume a stable scheduler order.
Provider-rejected unknown tools never reach a decorator and are outside this
trace's coverage. Document this limit rather than claiming full model reasoning
or request tracing.

## Runner lifecycle and isolation

1. Validate dataset, evaluator configurations, and runner options before any
   external execution. Resolve cases in dataset order.
2. Create an attempt ID, recorder, context deadline, and fresh target. Separate
   memory, conversation identity, mutable tool fixtures, and agent graph per
   attempt. Shared provider clients are allowed only if concurrency-safe.
3. Run `RunDetailed` exactly once. Preserve trace and elapsed time on error.
4. Close owned resources on success, failure, or cancellation with a separate
   bounded cleanup context. Freeze the observation after execution has ended.
5. Grade using a separate bounded context so an execution timeout can still be
   reported; suite cancellation stops further execution and grading.
6. Store results by dataset index, independent of completion order. Aggregate and
   export the report, including unstarted cases after cancellation.

Default to concurrency one and one attempt. Add bounded worker concurrency in
the runner phase. Do not automatically retry failed agent runs or repeat tools:
retries could change the measured behavior and duplicate side effects. Future
repeated trials must keep each attempt and report the distribution.

Context cancellation is cooperative: tools/providers must honor it. Do not claim
a hard timeout by abandoning an executing goroutine and closing its resources.
A noncooperative target can delay shutdown; process-level isolation is deferred.

Separate setup, execution, grading, and cleanup durations. The latency grader
uses execution duration only. A cleanup failure is recorded as an infrastructure
error. Never reuse a partially initialized target after a factory error.

## Deterministic graders

| Type | Contract |
| --- | --- |
| `output_exact` | Byte-equivalent text after configured normalization; default none, optional trim-space only. |
| `output_contains` | Case-sensitive substring after the same normalization rule. |
| `output_regex` | Go regexp matching, compiled at dataset validation. |
| `output_json_schema` | Output must contain one valid JSON value satisfying a precompiled schema; no implicit Markdown fence removal. |
| `tool_trajectory` | Exact ordered list, ordered subsequence, or unordered multiset; match names and optionally arguments at each expected call. |
| `max_tool_calls` | Compare observed call count within the selected layer and agent scope, preserving repeated calls. |
| `max_latency_ms` | Compare measured execution duration with the inclusive limit. |
| `max_total_tokens` | Compare reported total tokens only when coverage is sufficient; absent/partial usage is unavailable. |

For JSON arguments, object key order is irrelevant, array order is significant,
and numbers compare by exact numeric value without floating-point conversion.
Default argument matching is full structural equality. An explicit subset mode
allows extra object keys; arrays still match exactly. Non-JSON `Tool.Run` inputs
can use an explicitly configured raw-string matcher. Invalid observed arguments
fail a check; malformed expected arguments invalidate the dataset.

Built-in deterministic scores are 0 or 1. Every metric reports `pass`, `fail`,
`unavailable`, or `error`, a nullable score in [0,1], its threshold, and structured
evidence. A required unavailable/errored metric prevents a case from passing.
Execution failure always prevents success, even if the partial trace satisfies
some checks. Advisory checks (`required: false`) remain visible without changing
the gate. Reject cases containing only advisory checks in the initial release.

The default suite gate requires every selected case to pass. Report pass rate,
status counts, and per-metric means with explicit denominators; exclude null
scores from means and show unavailable counts. Do not collapse unrelated metrics
into a weighted score. Budget checks evaluate observations after execution;
they do not enforce a hard spending or tool-call limit.

## Reporting, regrading, and CLI

JSON reports have their own `schema_version: "1"`. Include dataset digest,
resolved evaluator configuration, SDK/build revision when available, user-supplied
configuration fingerprint, provider/model metadata, attempt IDs, capabilities,
durations, errors, observations, and results. Do not serialize the entire agent
configuration or arbitrary response metadata, which may contain credentials.

Default export contains metrics and bounded evidence. Full output/argument/tool
result capture is explicit and supports a caller-provided redaction callback.
Define configurable limits for dataset bytes, spans per attempt, and retained
content bytes; proposed defaults are 10 MiB, 10,000 spans, and 10 MiB respectively.
On overflow, mark the affected observation incomplete. Never grade truncated
data as complete. Grade before export redaction; redacted or omitted observations
cannot be faithfully regraded and must be marked as such on import.

Offline regrading takes complete saved observations, validates their version and
dataset/case identity, and calls `Grade` without creating a target. Unknown report
versions fail explicitly. A report-write failure must return an error; use a
temporary file and rename for file output.

JUnit emits one testcase per dataset case, with metric evidence in failures.
Use `<failure>` for unmet expectations, `<error>` for execution/evaluator/required
capability failures, and `<skipped>` for cases not started after cancellation.
XML-escape evidence and remove characters forbidden by XML 1.0.

Proposed CLI, implemented after the library:

```sh
agent-cli eval --dataset examples/evaluation/dataset.json \
  --config agent-config.json --format json --output evaluation.json
agent-cli eval --dataset examples/evaluation/dataset.json \
  --observations previous-full-report.json --format junit --output evaluation.xml
```

`--config` uses the existing CLI agent configuration format. Extract a shared
error-returning constructor accepting additional agent options so instrumentation
can be inserted without duplicating provider selection. Preserve existing command
behavior. Parse eval flags with `flag.FlagSet`; dispatch eval before the legacy
global direct-prompt scan. Inject output writers for tests and keep report stdout
free of banners/logs; diagnostics go to stderr.

CLI exit codes: 0 for a passing suite, 1 for unmet expectations, 2 for invalid
input or infrastructure/evaluation/reporting errors, and 130 for interrupt. Error
status takes precedence over ordinary failures; interrupted runs retain partial
reports where possible. Credentials remain in the existing environment/config
path. No network services are required for deterministic fixtures or regrading.

## Optional model judges

The opt-in `model_judge` evaluator uses an injected model and
`GenerateDetailed`. It requests a structured score and rationale, treats input,
reference, and agent output as untrusted evaluation data, and never gives the
judge tools or access to the evaluated agent's memory. Every response and score
range is validated; invalid output is an evaluation error with no hidden retry.

Record judge model, configuration, rubric version/hash, duration, and usage
separately from the evaluated agent. Grade thresholds and judge agreement need
calibration on a human-labeled fixture set before being used as release gates.
Cross-model results are not assumed comparable. Live judge tests are opt-in.

## Delivery phases and acceptance criteria

### PR 1: contracts and offline grading

- Add versioned types, strict JSON loading, config validation, and `Grade`.
- Implement text, JSON Schema, and tool trajectory graders with synthetic traces.
- Add canonical JSON report round-trip fixtures and a small example dataset.
- Acceptance: the same observation/config produces identical grades; invalid
  datasets fail before execution; nil, incomplete, and empty observations are
  distinct. No agent/provider changes or API keys are needed.

### PR 2: recorder and SDK adapter

- Implement correlated spans, capability reporting, limits, and SDK decorators.
- Add the factory helper with deliberate policy ordering and nested agent paths.
- Acceptance: overlapping identical tool calls remain distinct; both entry
  methods work; modified, short-circuited, failed, and recovered calls preserve
  their actual semantics; wrappers preserve optional interfaces. Race tests pass.

### PR 3: runner and resource graders

- Implement isolated factories, worker limits, deadlines, cleanup, aggregation,
  and latency/tool-count/token graders.
- Acceptance: case memory and fixtures do not leak, ordering is stable under
  concurrency, cancellation preserves partial results, nil/partial usage cannot
  pass a required token check, and no implicit reruns occur.

### PR 4: reporting and CLI

- Add JUnit, report import/regrading, redaction, atomic output, CLI integration,
  user documentation, and an offline CI example.
- Acceptance: parseable JSON/XML, correct exit codes, no stdout contamination,
  refusal to regrade incomplete data, and no changes to existing command behavior.

### PR 5: optional model judges (implemented)

- Add rubric-based evaluator with structured output validation and separate usage.
- Acceptance: malformed output is an error, judge data is isolated, configuration
  is recorded, and ordinary tests remain offline.

## Implementation validation

Use table-driven tests for schema/config validation and matcher edge cases;
property/fuzz tests for JSON argument comparison, strict loading, and report
round-trips. Include duplicate calls, exact numeric arguments, empty outputs,
truncation, and unknown versions. Use fake clocks where needed for aggregation
tests and controllable blocking fakes for cooperative timeout tests.

Integration tests should construct real `agent.Agent` instances with fake LLMs
and tools. Cover both `Execute` and `Run` dispatch, policy ordering, nested agent
capture, failed runs with nil responses, and concurrent case isolation. Fake
provider tests validate the SDK integration seam; they do not prove every live
provider's usage accounting. Keep optional live smoke tests separate.

After each implementation phase, run focused tests and the existing checks
relevant to modified packages. Before merging the implementation series:

```sh
go test ./pkg/eval/... ./pkg/agent/... ./pkg/hooks/...
go test -race ./pkg/eval/...
go test ./cmd/agent-cli/...
go test ./...
```

These commands validate the implemented packages and CLI integration.
