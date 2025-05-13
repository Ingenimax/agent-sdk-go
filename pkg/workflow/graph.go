package workflow

// NodeType represents the type of a node in the workflow graph
type NodeType string

const (
	NodeTypeAgent     NodeType = "agent"
	NodeTypeCondition NodeType = "condition"
	NodeTypeEnd       NodeType = "end"
)

// GraphNode represents a node in the workflow graph
type GraphNode struct {
	ID         string
	Type       NodeType
	AgentID    string
	OutputType interface{}
	Edges      []Edge
	Input      string
	Output     interface{}
	Status     string
}

// Edge represents a connection between nodes in the workflow graph
type Edge struct {
	From        string
	To          string
	Condition   func(*WorkflowState) bool
	DataMapping map[string]string
}

// WorkflowState represents the current state of the workflow execution
type WorkflowState struct {
	CurrentNode string
	Variables   map[string]interface{}
	NodeOutputs map[string]interface{}
	History     []StateTransition
	Error       error
	Workflow    *GraphWorkflow
}

// StateTransition represents a transition between nodes in the workflow
type StateTransition struct {
	FromNode  string
	ToNode    string
	Timestamp int64
	Variables map[string]interface{}
	Output    interface{}
}

// GraphWorkflow represents a workflow defined as a graph of nodes
type GraphWorkflow struct {
	Nodes     map[string]*GraphNode
	StartNode string
	Input     string
}

// NewGraphWorkflow creates a new graph workflow
func NewGraphWorkflow() *GraphWorkflow {
	return &GraphWorkflow{
		Nodes: make(map[string]*GraphNode),
	}
}

// AddNode adds a node to the workflow
func (w *GraphWorkflow) AddNode(node *GraphNode) {
	w.Nodes[node.ID] = node
	if w.StartNode == "" {
		w.StartNode = node.ID
	}
}

// AddEdge adds an edge between two nodes
func (w *GraphWorkflow) AddEdge(from, to string, condition func(*WorkflowState) bool, dataMapping map[string]string) {
	fromNode, exists := w.Nodes[from]
	if !exists {
		return
	}

	edge := Edge{
		From:        from,
		To:          to,
		Condition:   condition,
		DataMapping: dataMapping,
	}

	fromNode.Edges = append(fromNode.Edges, edge)
}

// SetInput sets the input for the workflow
func (w *GraphWorkflow) SetInput(input string) {
	w.Input = input
}
