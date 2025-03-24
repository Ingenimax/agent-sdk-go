package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/executionplan"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/tools/calculator"
	"github.com/google/uuid"
)

// Server represents the API server
type Server struct {
	router           *http.ServeMux
	agents           map[string]*agent.Agent
	conversations    map[string]*Conversation
	conversationLock sync.RWMutex
}

// Conversation represents a conversation with an agent
type Conversation struct {
	ID        string
	UserID    string
	Messages  []Message
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message represents a message in a conversation
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// ChatRequest represents a request to the chat endpoint
type ChatRequest struct {
	UserID         string `json:"user_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	Message        string `json:"message"`
}

// ChatResponse represents a response from the chat endpoint
type ChatResponse struct {
	ConversationID string  `json:"conversation_id"`
	Message        Message `json:"message"`
	TaskID         string  `json:"task_id,omitempty"`
}

// TaskResponse represents a response from the task endpoint
type TaskResponse struct {
	ID          string                   `json:"id"`
	Status      string                   `json:"status"`
	Description string                   `json:"description"`
	Steps       []map[string]interface{} `json:"steps"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

// NewServer creates a new API server
func NewServer() *Server {
	s := &Server{
		router:        http.NewServeMux(),
		agents:        make(map[string]*agent.Agent),
		conversations: make(map[string]*Conversation),
	}

	// Set up routes
	s.router.HandleFunc("/api/v1/chat", s.handleChat)
	s.router.HandleFunc("/api/v1/tasks/", s.handleTasks)

	return s
}

// Run starts the server
func (s *Server) Run(addr string) error {
	log.Printf("Server listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

// handleChat handles chat requests
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	// Get or create conversation
	s.conversationLock.Lock()
	var conversation *Conversation
	if req.ConversationID != "" {
		conversation = s.conversations[req.ConversationID]
		if conversation == nil {
			s.conversationLock.Unlock()
			http.Error(w, "Conversation not found", http.StatusNotFound)
			return
		}
	} else {
		conversationID := uuid.New().String()
		conversation = &Conversation{
			ID:        conversationID,
			UserID:    req.UserID,
			Messages:  []Message{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		s.conversations[conversationID] = conversation
	}
	s.conversationLock.Unlock()

	// Get or create agent for this user
	agentInstance, err := s.getOrCreateAgent(req.UserID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create agent: %v", err), http.StatusInternalServerError)
		return
	}

	// Add user message to conversation
	userMessage := Message{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	s.conversationLock.Lock()
	conversation.Messages = append(conversation.Messages, userMessage)
	conversation.UpdatedAt = time.Now()
	s.conversationLock.Unlock()

	// Process the message with the agent
	ctx := context.Background()
	response, err := agentInstance.Run(ctx, req.Message)
	if err != nil {
		http.Error(w, fmt.Sprintf("Agent error: %v", err), http.StatusInternalServerError)
		return
	}

	// Add agent message to conversation
	agentMessage := Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	}
	s.conversationLock.Lock()
	conversation.Messages = append(conversation.Messages, agentMessage)
	conversation.UpdatedAt = time.Now()
	s.conversationLock.Unlock()

	// Check if the response contains a task ID
	var taskID string
	for _, plan := range agentInstance.ListTasks() {
		if plan.Status == executionplan.StatusPendingApproval {
			taskID = plan.TaskID
			break
		}
	}

	// Create response
	resp := ChatResponse{
		ConversationID: conversation.ID,
		Message:        agentMessage,
		TaskID:         taskID,
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleTasks handles requests to get tasks
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from URL if present
	path := r.URL.Path
	taskID := ""
	if len(path) > len("/api/v1/tasks/") {
		taskID = path[len("/api/v1/tasks/"):]
	}

	if taskID != "" {
		// Handle get task by ID
		s.handleGetTask(w, r, taskID)
	} else {
		// Handle list tasks
		s.handleListTasks(w, r)
	}
}

// handleGetTask handles requests to get a task
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Find the agent that has this task
	var taskPlan *executionplan.ExecutionPlan
	var found bool
	for _, agentInstance := range s.agents {
		if plan, exists := agentInstance.GetTaskByID(taskID); exists {
			taskPlan = plan
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Convert steps to a format suitable for JSON
	steps := make([]map[string]interface{}, len(taskPlan.Steps))
	for i, step := range taskPlan.Steps {
		steps[i] = map[string]interface{}{
			"tool_name":   step.ToolName,
			"description": step.Description,
			"input":       step.Input,
			"parameters":  step.Parameters,
		}
	}

	// Create response
	resp := TaskResponse{
		ID:          taskPlan.TaskID,
		Status:      string(taskPlan.Status),
		Description: taskPlan.Description,
		Steps:       steps,
		CreatedAt:   taskPlan.CreatedAt,
		UpdatedAt:   taskPlan.UpdatedAt,
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleListTasks handles requests to list all tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Collect all tasks from all agents
	var tasks []TaskResponse
	for _, agentInstance := range s.agents {
		for _, plan := range agentInstance.ListTasks() {
			// Convert steps to a format suitable for JSON
			steps := make([]map[string]interface{}, len(plan.Steps))
			for i, step := range plan.Steps {
				steps[i] = map[string]interface{}{
					"tool_name":   step.ToolName,
					"description": step.Description,
					"input":       step.Input,
					"parameters":  step.Parameters,
				}
			}

			tasks = append(tasks, TaskResponse{
				ID:          plan.TaskID,
				Status:      string(plan.Status),
				Description: plan.Description,
				Steps:       steps,
				CreatedAt:   plan.CreatedAt,
				UpdatedAt:   plan.UpdatedAt,
			})
		}
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// getOrCreateAgent gets or creates an agent for a user
func (s *Server) getOrCreateAgent(userID string) (*agent.Agent, error) {
	// Check if agent already exists
	if agentInstance, exists := s.agents[userID]; exists {
		return agentInstance, nil
	}

	// Create tools
	toolRegistry := tools.NewRegistry()

	// Register some tools
	calcTool := calculator.NewCalculator()
	toolRegistry.Register(calcTool)

	// Register some simple tools for demonstration
	toolRegistry.Register(&SimpleTool{
		name:        "aws_deploy",
		description: "Deploy resources to AWS",
	})

	toolRegistry.Register(&SimpleTool{
		name:        "aws_configure",
		description: "Configure AWS resources",
	})

	// Create a mock LLM
	mockLLM := &MockLLM{}

	// Create the agent
	agentInstance, err := agent.NewAgent(
		agent.WithLLM(mockLLM),
		agent.WithTools(toolRegistry.List()...),
		agent.WithRequirePlanApproval(true),
	)
	if err != nil {
		return nil, err
	}

	// Store the agent
	s.agents[userID] = agentInstance

	return agentInstance, nil
}

// SimpleTool is a basic tool for demonstration purposes
type SimpleTool struct {
	name        string
	description string
}

// Name returns the name of the tool
func (t *SimpleTool) Name() string {
	return t.name
}

// Description returns a description of what the tool does
func (t *SimpleTool) Description() string {
	return t.description
}

// Run executes the tool with the given input
func (t *SimpleTool) Run(ctx context.Context, input string) (string, error) {
	return fmt.Sprintf("Executed %s with input: %s", t.name, input), nil
}

// Parameters returns the parameters that the tool accepts
func (t *SimpleTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"region": {
			Type:        "string",
			Description: "AWS region",
			Required:    true,
		},
		"instance_type": {
			Type:        "string",
			Description: "EC2 instance type",
			Required:    false,
			Default:     "t2.micro",
		},
	}
}

// Execute executes the tool with the given arguments
func (t *SimpleTool) Execute(ctx context.Context, args string) (string, error) {
	return fmt.Sprintf("Executed %s with arguments: %s", t.name, args), nil
}

// MockLLM is a mock implementation of the LLM interface for demonstration purposes
type MockLLM struct{}

// Generate generates text based on the provided prompt
func (m *MockLLM) Generate(ctx context.Context, prompt string, options ...interfaces.GenerateOption) (string, error) {
	// For demonstration purposes, return a simple execution plan
	if strings.Contains(prompt, "Create an execution plan") {
		return `{
			"description": "Deploy a Node.js application on AWS with load balancer and auto-scaling",
			"steps": [
				{
					"toolName": "aws_configure",
					"description": "Configure AWS VPC and networking",
					"input": "Create VPC with public and private subnets in us-west-2",
					"parameters": {
						"region": "us-west-2"
					}
				},
				{
					"toolName": "aws_deploy",
					"description": "Deploy Node.js application with auto-scaling",
					"input": "Deploy Node.js app with auto-scaling based on CPU usage (>70%)",
					"parameters": {
						"region": "us-west-2",
						"instance_type": "t2.micro"
					}
				}
			]
		}`, nil
	}

	// For plan modification, return a modified plan
	if strings.Contains(prompt, "modify the execution plan") {
		return `{
			"description": "Deploy a Node.js application on AWS with load balancer, auto-scaling, and CloudFront",
			"steps": [
				{
					"toolName": "aws_configure",
					"description": "Configure AWS VPC and networking",
					"input": "Create VPC with public and private subnets in us-west-2",
					"parameters": {
						"region": "us-west-2"
					}
				},
				{
					"toolName": "aws_deploy",
					"description": "Deploy Node.js application with auto-scaling",
					"input": "Deploy Node.js app with auto-scaling based on CPU usage (>70%)",
					"parameters": {
						"region": "us-west-2",
						"instance_type": "t3.medium"
					}
				},
				{
					"toolName": "aws_configure",
					"description": "Set up CloudFront for content delivery",
					"input": "Configure CloudFront distribution for the application",
					"parameters": {
						"region": "us-west-2"
					}
				}
			]
		}`, nil
	}

	// For general conversation
	return "I'll help you with that. Let me create an execution plan for deploying your application.", nil
}

// GenerateWithTools generates text and can use tools
func (m *MockLLM) GenerateWithTools(ctx context.Context, prompt string, tools []interfaces.Tool, options ...interfaces.GenerateOption) (string, error) {
	return m.Generate(ctx, prompt, options...)
}

// Name returns the name of the LLM provider
func (m *MockLLM) Name() string {
	return "MockLLM"
}

func main() {
	// Create and run the server
	server := NewServer()
	log.Fatal(server.Run(":8080"))
}
