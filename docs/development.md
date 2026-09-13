# Development Guide

This document provides detailed information for developers contributing to the Agent SDK for Go.

## Setting Up Your Development Environment

### Prerequisites

- Go 1.26+
- Git
- An IDE with Go support (VSCode, GoLand, etc.)
- pre-commit (optional but recommended)

### Getting Started

1. Clone the repository:
   ```bash
   git clone https://github.com/Ingenimax/agent-sdk-go.git
   cd agent-sdk-go
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set up pre-commit hooks:
   ```bash
   ./scripts/dev-env-setup.sh
   ```

## Code Quality Tools

### Pre-commit Hooks

We use pre-commit to ensure code quality. Our pre-commit configuration includes:

- Basic file checks (trailing whitespace, end-of-file fixing, YAML validation)
- Go-specific checks (go-fmt, go-imports, go unit tests, go mod tidy)
- golangci-lint for comprehensive linting
- gosec for Go security scanning

The hooks run automatically on each commit, but you can also run them manually:
```bash
# Run on all files
pre-commit run --all-files

# Run a specific hook
pre-commit run go-fmt
```

### Golangci-lint

For more advanced linting, we use golangci-lint. The pre-commit configuration uses a basic setup, but you can create a custom `.golangci.yml` file in the project root for more specific linting rules.

Example golangci-lint configuration:
```yaml
# .golangci.yml example
run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
    - misspell
```

For more configuration options, see the [golangci-lint documentation](https://golangci-lint.run/usage/configuration/).

To run golangci-lint manually:
```bash
golangci-lint run
```

### Gosec - Go Security Scanner

Gosec is a security scanner for Go code. It identifies potential security issues by checking against a set of rules. Our pre-commit configuration includes gosec with some basic exclusions.

You can run gosec manually:
```bash
# Run on all packages
gosec ./...

# Run with specific rules excluded
gosec -exclude=G104,G307 ./...

# Output in JSON format
gosec -fmt=json -out=results.json ./...
```

Common security issues that gosec identifies:
- Hardcoded credentials
- Unsafe uses of the `os/exec` package
- SQL injection vulnerabilities
- Weak cryptographic primitives

For more information, see the [Gosec documentation](https://github.com/securego/gosec).

## Testing

Please write tests for any new functionality:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test github.com/Ingenimax/agent-sdk-go/pkg/agent
```

## Documentation

Keep documentation up-to-date when making changes. This includes:

1. Code comments (especially on exported functions)
2. README.md updates
3. Documentation in the docs/ directory

## Pull Request Process

1. Create a feature branch for your changes
2. Make your changes and ensure they are well-tested
3. Run pre-commit hooks to check for any issues
4. Submit a pull request to the main branch
5. Request a review from the appropriate team based on our CODEOWNERS file

## Additional Resources

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://golang.org/doc/effective_go)
- [Go Modules](https://blog.golang.org/using-go-modules)

## Test doubles

`internal/testutil` provides shared, concurrency-safe test doubles. Use these
rather than hand-rolling a mock per test file:

| Double | Implements |
| --- | --- |
| `FakeLLM` | `interfaces.LLM` and `interfaces.StreamingLLM` |
| `FakeTool` | `interfaces.Tool`, `ToolWithDisplayName`, `InternalTool` |
| `FakeMemory` | `interfaces.Memory` |
| `FakeConversationMemory` | `interfaces.ConversationMemory` |
| `FakeTracer`, `FakeSpan` | `interfaces.Tracer`, `interfaces.Span` |

```go
import "github.com/Ingenimax/agent-sdk-go/internal/testutil"

llm := &testutil.FakeLLM{
    Responses:       []string{"first answer", "second answer"},
    DefaultResponse: "fallback",
    Usage:           &interfaces.TokenUsage{InputTokens: 10, OutputTokens: 5},
}

agent, err := agent.NewAgent(
    agent.WithLLM(llm),
    agent.WithTools(&testutil.FakeTool{ToolName: "search", Result: "results"}),
)

// Afterwards
llm.Calls()          // how many Generate* calls
llm.Prompts()        // every prompt, in order
llm.ToolNamesAt(0)   // tools offered on the first call
```

`testutil.NewFakeLLM()` returns one that answers everything with `"ok"`.

Take full control with `GenerateFunc`, inject failures with `Err`, and drive the
tool set with `InvokeTools: true`.

All doubles are safe to call from multiple goroutines, so a race the detector
reports under `go test -race` is a race in the code under test rather than in
the double. That was not true of the mocks this package replaced.

`internal/testutil` is under `internal/` deliberately: it is a convenience for
this module's tests, not API supported for downstream modules.
