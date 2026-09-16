# Agent evaluation

The evaluation framework runs independent agent cases, records observable tool
behavior, and grades the resulting observations without depending on a hosted
evaluation service. It supports live execution and offline regrading.

## Dataset format

Datasets are strict JSON. Unknown fields, duplicate object keys, duplicate case
or check IDs, invalid evaluator configuration, and unsupported schema versions
are rejected before an agent is constructed.

```json
{
  "schema_version": "1",
  "id": "support-agent",
  "cases": [
    {
      "id": "refund-policy",
      "input": "Can I return an item after 14 days?",
      "checks": [
        {
          "id": "answer",
          "type": "output_contains",
          "config": {"value": "14 days"}
        },
        {
          "id": "search-path",
          "type": "tool_trajectory",
          "config": {
            "layer": "execution",
            "mode": "exact",
            "calls": [
              {"name": "policy_search", "arguments": {"topic": "returns"}}
            ]
          }
        }
      ]
    }
  ]
}
```

Every check has a score threshold of `1` and is required by default. Advisory
checks set `"required": false`; each case must still contain at least one
required check.

The built-in check types are:

| Type | Configuration |
| --- | --- |
| `output_exact` | `value`; optional `normalization`: `none` or `trim_space` |
| `output_contains` | `value`; optional `normalization` |
| `output_regex` | Go `pattern`; optional `normalization` |
| `output_json_schema` | local `schema`; remote `$ref` loading is disabled |
| `tool_trajectory` | `calls`; optional `layer`, `agent_path`, `mode`, and `argument_match` |
| `max_tool_calls` | `maximum`; optional `layer` and `agent_path` |
| `max_latency_ms` | `maximum` |
| `max_total_tokens` | `maximum`; requires complete provider usage data |

Trajectory modes are `exact`, `subsequence`, and `unordered`. Argument matching
is `full`, `subset`, or `raw`. JSON object key order is ignored, arrays retain
their order, and numbers compare by exact numeric value.

## Library integration

A target factory must construct fresh mutable state for every case. Add the SDK
adapter's options when constructing each agent:

```go
runner := eval.Runner{
    Factory: func(ctx context.Context, c eval.Case, recorder *eval.Recorder) (eval.Target, error) {
        options := []agent.Option{
            agent.WithLLM(newLLM()),
            agent.WithTools(newTools()...),
            agent.WithRequirePlanApproval(false),
        }
        options = append(options, evalsdk.Options(recorder, eval.RootAgentPath)...)

        subject, err := agent.NewAgent(options...)
        if err != nil {
            return eval.Target{}, err
        }
        return evalsdk.NewTarget(subject, nil), nil
    },
}

report, err := runner.Run(ctx, dataset)
```

`evalsdk.Options` installs an outer attempt recorder and an inner execution
recorder. Policy decorators can be passed between them:

```go
options = append(options,
    evalsdk.Options(recorder, eval.RootAgentPath, policies.Decorate("root"))...,
)
```

Attempt spans contain the model-supplied arguments and result returned to the
model. Execution spans contain post-policy arguments and the actual tool result.
A denied call has an attempt span marked `short_circuited` and no execution span.
Checks select one layer and never count both.

The runner defaults to sequential execution. `RunnerOptions.Concurrency` enables
bounded parallel cases, while the result stays in dataset order. Each case runs
once; there are no automatic retries. Timeouts depend on provider and tool code
honoring context cancellation. Output, tool, and usage coverage are explicit;
required checks become unavailable when their input was truncated or could not
be observed completely. Target factories and custom evaluators must be safe for
concurrent calls when concurrency is greater than one.

## CLI

Run a dataset with the configured provider:

```sh
agent-cli eval \
  --dataset examples/evaluation/dataset.json \
  --config ~/.agent-cli/config.json \
  --format json \
  --output evaluation.json
```

For an OpenAI-compatible endpoint such as OpenRouter, keep `provider` set to
`openai`, set the endpoint's model in the CLI config, and provide both
environment variables:

```sh
export OPENAI_API_KEY="$OPENROUTER_API_KEY"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
agent-cli eval --dataset dataset.json --config config.json
```

JSON exports omit full observations by default. Include them when the report
will be regraded later:

```sh
agent-cli eval \
  --dataset examples/evaluation/dataset.json \
  --include-observations \
  --output observations.json

agent-cli eval \
  --dataset examples/evaluation/dataset.json \
  --observations observations.json \
  --format junit \
  --output evaluation.xml
```

Exit code `0` means all cases passed, `1` means one or more expectations failed,
`2` means execution, evaluation, configuration, or report output failed, and
`130` means the suite was interrupted. Reports are still written after a partial
interrupted run when possible.

The complete offline example requires no provider credentials:

```sh
go run ./examples/evaluation
```

## Reports and privacy

`eval.WriteJSON` and `eval.WriteJSONFile` write versioned reports. The default
export removes prompts' resulting output and tool arguments/results, which makes
the saved observations non-regradable. `ExportOptions.IncludeObservations` keeps
them. A redaction callback or export truncation also marks affected observations
non-regradable.

`eval.WriteJUnit` emits one testcase per case. Failed expectations use JUnit
failures; execution errors, evaluator errors, and required missing capabilities
use JUnit errors. Cases left unstarted after cancellation are skipped.

The framework records observable calls at tool decorators. It does not expose a
provider's hidden reasoning, provider-rejected unknown tool calls, or internals
of remote agents that were not instrumented. Such coverage must be declared
partial or unavailable so required checks cannot pass on missing data.
