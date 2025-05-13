package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// ExecutionOptions represents options for workflow execution
type ExecutionOptions struct {
	Input      string
	Timeout    time.Duration
	MaxRetries int
}

// WorkflowExecutor executes a graph workflow
type WorkflowExecutor struct {
	registry *agent.Registry
	logger   interfaces.Logger
}

// NewWorkflowExecutor creates a new workflow executor
func NewWorkflowExecutor(registry *agent.Registry) *WorkflowExecutor {
	return &WorkflowExecutor{
		registry: registry,
	}
}

// ExecuteGraph executes a graph workflow
func (e *WorkflowExecutor) ExecuteGraph(ctx context.Context, workflow *GraphWorkflow, opts ExecutionOptions) (*WorkflowState, error) {
	state := &WorkflowState{
		Variables:   make(map[string]interface{}),
		NodeOutputs: make(map[string]interface{}),
		History:     make([]StateTransition, 0),
		Workflow:    workflow,
	}

	// Set initial input
	if opts.Input != "" {
		workflow.SetInput(opts.Input)
	}

	currentNode := workflow.Nodes[workflow.StartNode]
	if currentNode == nil {
		return state, fmt.Errorf("start node not found")
	}

	// Set input for the first node
	currentNode.Input = opts.Input

	for currentNode != nil {
		// Execute the current node
		output, err := e.executeNode(ctx, currentNode, state)
		if err != nil {
			state.Error = fmt.Errorf("node execution failed: %w", err)
			return state, state.Error
		}

		// Store node output
		state.NodeOutputs[currentNode.ID] = output

		// Find next node based on edges and conditions
		nextNode := e.determineNextNode(currentNode, state)
		if nextNode == nil {
			break
		}

		// Record transition
		transition := StateTransition{
			FromNode:  currentNode.ID,
			ToNode:    nextNode.ID,
			Timestamp: time.Now().Unix(),
			Variables: state.Variables,
			Output:    output,
		}
		state.History = append(state.History, transition)

		// Map output data to next node's input
		e.mapNodeData(currentNode, nextNode, state)

		currentNode = nextNode
	}

	return state, nil
}

// executeNode executes a single node in the workflow
func (e *WorkflowExecutor) executeNode(ctx context.Context, node *GraphNode, state *WorkflowState) (interface{}, error) {
	switch node.Type {
	case NodeTypeAgent:
		return e.executeAgentNode(ctx, node, state)
	case NodeTypeCondition:
		return e.executeConditionNode(ctx, node, state)
	case NodeTypeEnd:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown node type: %s", node.Type)
	}
}

// executeAgentNode executes an agent node
func (e *WorkflowExecutor) executeAgentNode(ctx context.Context, node *GraphNode, state *WorkflowState) (interface{}, error) {
	agent, ok := e.registry.Get(node.AgentID)
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", node.AgentID)
	}

	result, err := agent.Run(ctx, node.Input)
	if err != nil {
		return nil, fmt.Errorf("agent execution failed: %w", err)
	}

	return result, nil
}

// executeConditionNode executes a condition node
func (e *WorkflowExecutor) executeConditionNode(ctx context.Context, node *GraphNode, state *WorkflowState) (interface{}, error) {
	// For condition nodes, we just return the current state
	return state, nil
}

// determineNextNode determines the next node to execute based on edges and conditions
func (e *WorkflowExecutor) determineNextNode(currentNode *GraphNode, state *WorkflowState) *GraphNode {
	for _, edge := range currentNode.Edges {
		// If no condition or condition evaluates to true
		if edge.Condition == nil || edge.Condition(state) {
			return state.Workflow.Nodes[edge.To]
		}
	}
	return nil
}

// mapNodeData maps output data from one node to the input of another
func (e *WorkflowExecutor) mapNodeData(fromNode, toNode *GraphNode, state *WorkflowState) {
	for _, edge := range fromNode.Edges {
		if edge.To == toNode.ID {
			// Apply data mapping
			if output, ok := state.NodeOutputs[fromNode.ID]; ok {
				// TODO: Implement proper data mapping using reflection
				// For now, just pass the entire output
				toNode.Input = fmt.Sprintf("%v", output)
			}
			break
		}
	}
}
