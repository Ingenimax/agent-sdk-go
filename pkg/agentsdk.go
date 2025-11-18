package agentsdk

import (
	"context"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/agentconfig"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/Ingenimax/agent-sdk-go/pkg/task"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/api"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/executor"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/planner"
	"github.com/Ingenimax/agent-sdk-go/pkg/task/service"
)

// NewAgent creates a new agent with the given options
func NewAgent(options ...agent.Option) (*agent.Agent, error) {
	return agent.NewAgent(options...)
}

// WithLLM sets the LLM for the agent
func WithLLM(llm interfaces.LLM) agent.Option {
	return agent.WithLLM(llm)
}

// WithMemory sets the memory for the agent
func WithMemory(memory interfaces.Memory) agent.Option {
	return agent.WithMemory(memory)
}

// WithTools sets the tools for the agent
func WithTools(tools ...interfaces.Tool) agent.Option {
	return agent.WithTools(tools...)
}

// WithOrgID sets the organization ID for multi-tenancy
func WithOrgID(orgID string) agent.Option {
	return agent.WithOrgID(orgID)
}

// WithTracer sets the tracer for the agent
func WithTracer(tracer interfaces.Tracer) agent.Option {
	return agent.WithTracer(tracer)
}

// WithGuardrails sets the guardrails for the agent
func WithGuardrails(guardrails interfaces.Guardrails) agent.Option {
	return agent.WithGuardrails(guardrails)
}

// Task Execution

// NewTaskExecutor creates a new task executor
func NewTaskExecutor() *executor.TaskExecutor {
	return executor.NewTaskExecutor()
}

// NewAPIClient creates a new API client for making API calls
func NewAPIClient(baseURL string, timeout time.Duration) *api.Client {
	return api.NewClient(baseURL, timeout)
}

// NewTaskService creates a new task service with in-memory storage
func NewTaskService(logger logging.Logger) interfaces.TaskService {
	taskPlanner := planner.NewCorePlanner(logger)
	return service.NewCoreMemoryService(logger, taskPlanner)
}

// NewTaskAPI creates a new task API client
func NewTaskAPI(client *api.Client) *api.TaskAPI {
	return api.NewTaskAPI(client)
}

// Creates a new agent task service
func NewAgentTaskService(logger logging.Logger) (*task.AgentTaskService, error) {
	return task.NewAgentTaskService(logger)
}

// Creates a new agent task service with a custom adapter
func NewAgentTaskServiceWithAdapter(logger logging.Logger, service task.AgentAdapterService) *task.AgentTaskService {
	return task.NewAgentTaskServiceWithAdapter(logger, service)
}

// Configuration Management

// NewConfigClient creates a new configuration client for fetching deployment configurations
func NewConfigClient() (*agentconfig.ConfigurationClient, error) {
	return agentconfig.NewClient()
}

// LoadDeploymentConfig loads configuration from environment variables
// Reads AGENT_DEPLOYMENT_ID and ENVIRONMENT and fetches config from StarOps config service
func LoadDeploymentConfig(ctx context.Context) (map[string]string, error) {
	return agentconfig.LoadFromEnvironment(ctx)
}
