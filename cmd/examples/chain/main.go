package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Ingenimax/agent-sdk-go/cmd/examples/chain/tools"
	"github.com/Ingenimax/agent-sdk-go/pkg/agent"
	"github.com/Ingenimax/agent-sdk-go/pkg/config"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/llm/openai"
	"github.com/Ingenimax/agent-sdk-go/pkg/structuredoutput"
	"github.com/Ingenimax/agent-sdk-go/pkg/workflow"
)

var openaiClient interfaces.LLM

// Define structured output types for each agent
type TerraformPlan struct {
	ModuleName string   `json:"module_name" description:"Name of the Terraform module"`
	Resources  []string `json:"resources" description:"List of resources to be created"`
	Variables  []string `json:"variables" description:"List of variables needed"`
	Outputs    []string `json:"outputs" description:"List of outputs to be defined"`
}

type FileGenerationPlan struct {
	Files []FileSpec `json:"files" description:"List of files to be generated"`
}

type FileSpec struct {
	Name    string `json:"name" description:"Name of the file"`
	Content string `json:"content" description:"Content of the file"`
	Type    string `json:"type" description:"Type of file (main.tf, variables.tf, etc)"`
}

type ReviewResult struct {
	Approved          bool     `json:"approved" description:"Whether the file was approved"`
	Comments          []string `json:"comments" description:"Review comments"`
	Suggestions       []string `json:"suggestions" description:"Suggested improvements"`
	ValidationResults string   `json:"validation_results" description:"Results from terraform validate"`
}

type FileWriteResult struct {
	Success bool   `json:"success" description:"Whether the file was written successfully"`
	Path    string `json:"path" description:"Path where the file was written"`
	Error   string `json:"error,omitempty" description:"Error message if any"`
}

// Add max retries constant
const maxRetries = 10

func main() {
	log.Printf("Starting Terraform module generation workflow")

	// Initialize configuration
	cfg := config.LoadFromEnv()
	if cfg.LLM.OpenAI.APIKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}
	log.Printf("Configuration loaded successfully")

	// Initialize OpenAI client with response format enforcement
	openaiClient = openai.NewClient(cfg.LLM.OpenAI.APIKey,
		openai.WithModel("gpt-4o-mini"),
	)
	log.Printf("OpenAI client initialized with model: gpt-4o-mini")

	// Set static input for Terraform module requirements
	userInput := `Create a module for deploying a EKS cluster with the following requirements:
	- Create a VPC
	- Create a EKS cluster
	- Create a EKS node group
	- Create a EKS cluster security group
	- Create a EKS cluster security group rule
	- Create a EKS cluster security group rule
	`
	log.Printf("User input received: %s", userInput)
	// Create a new graph workflow
	log.Printf("Creating new graph workflow")
	graphWorkflow := workflow.NewGraphWorkflow()

	// Create the file writer tool
	fileWriter := &tools.FileWriterTool{}

	// Create the terraform validate tool with reference to file writer
	terraformValidate := &tools.TerraformValidateTool{
		FileWriter: fileWriter,
	}

	// Create nodes for each agent
	log.Printf("Creating workflow nodes")
	plannerNode := &workflow.GraphNode{
		ID:         "planner",
		Type:       workflow.NodeTypeAgent,
		AgentID:    "terraform_planner",
		OutputType: TerraformPlan{},
	}

	generatorNode := &workflow.GraphNode{
		ID:         "generator",
		Type:       workflow.NodeTypeAgent,
		AgentID:    "file_generator",
		OutputType: FileGenerationPlan{},
	}

	reviewerNode := &workflow.GraphNode{
		ID:         "reviewer",
		Type:       workflow.NodeTypeAgent,
		AgentID:    "file_reviewer",
		OutputType: ReviewResult{},
	}

	// Add nodes to workflow
	log.Printf("Adding nodes to workflow")
	graphWorkflow.AddNode(plannerNode)
	graphWorkflow.AddNode(generatorNode)
	graphWorkflow.AddNode(reviewerNode)

	// Add edges with data mapping
	log.Printf("Adding workflow edges")
	graphWorkflow.AddEdge("planner", "generator", nil, map[string]string{
		"module_name": "module_name",
		"resources":   "resources",
		"variables":   "variables",
		"outputs":     "outputs",
	})

	graphWorkflow.AddEdge("generator", "reviewer", nil, map[string]string{
		"files": "files_to_review",
	})

	// Add retry counter to state
	retryCount := 0

	// Add conditional edges for reviewer
	isApproved := func(state *workflow.WorkflowState) bool {
		log.Printf("Checking if review is approved (Attempt %d/%d)", retryCount+1, maxRetries)
		if reviewerOutput, ok := state.NodeOutputs["reviewer"].(string); ok {
			log.Printf("Reviewer raw output: %s", reviewerOutput)
			var review ReviewResult
			if err := json.Unmarshal([]byte(reviewerOutput), &review); err == nil {
				log.Printf("Review successfully unmarshaled: %+v", review)
				log.Printf("Review approval status: %v", review.Approved)
				return review.Approved
			} else {
				log.Printf("Failed to unmarshal reviewer output: %v", err)
				log.Printf("Raw reviewer output that failed to unmarshal: %s", reviewerOutput)
			}
		} else {
			log.Printf("No reviewer output found in state")
		}
		return false
	}

	// If approved, end the workflow
	graphWorkflow.AddEdge("reviewer", "end", isApproved, nil)

	// If not approved, go back to generator with retry limit
	graphWorkflow.AddEdge("reviewer", "generator", func(state *workflow.WorkflowState) bool {
		if retryCount >= maxRetries {
			log.Printf("Maximum retry attempts (%d) reached. Ending workflow.", maxRetries)
			return false
		}
		retryCount++
		approved := !isApproved(state)
		log.Printf("Review not approved, returning to generator (Attempt %d/%d): %v", retryCount, maxRetries, approved)
		return approved
	}, map[string]string{
		"review_comments":    "review_comments",
		"review_suggestions": "review_suggestions",
	})

	// Create agent registry
	log.Printf("Creating agent registry")
	registry := agent.NewRegistry()

	// Register agents
	log.Printf("Registering agents")
	registry.Register("terraform_planner", createPlannerAgent())
	registry.Register("file_generator", createGeneratorAgent(fileWriter))
	registry.Register("file_reviewer", createReviewerAgent(terraformValidate))

	// Create workflow executor
	log.Printf("Creating workflow executor")
	executor := workflow.NewWorkflowExecutor(registry)

	// Execute workflow with input
	log.Printf("Starting workflow execution")
	ctx := context.Background()
	state, err := executor.ExecuteGraph(ctx, graphWorkflow, workflow.ExecutionOptions{
		Input: fmt.Sprintf("Create a Terraform module with the following requirements:\n%s", userInput),
	})
	if err != nil {
		log.Fatalf("Workflow execution failed: %v", err)
	}

	// Print results
	log.Printf("Workflow completed successfully!")
	log.Printf("Processing workflow results")

	log.Printf("\nGenerator output:")
	if generatorOutput, ok := state.NodeOutputs["generator"].(string); ok {
		var files FileGenerationPlan
		if err := json.Unmarshal([]byte(generatorOutput), &files); err == nil {
			for _, file := range files.Files {
				log.Printf("\nFile: %s (%s)", file.Name, file.Type)
				log.Printf("Content length: %d bytes", len(file.Content))
			}
		} else {
			log.Printf("Failed to unmarshal generator output: %v", err)
		}
	}

	log.Printf("\nReviewer output:")
	if reviewerOutput, ok := state.NodeOutputs["reviewer"].(string); ok {
		var review ReviewResult
		if err := json.Unmarshal([]byte(reviewerOutput), &review); err == nil {
			log.Printf("Approved: %v", review.Approved)
			log.Printf("Comments: %v", review.Comments)
			log.Printf("Suggestions: %v", review.Suggestions)
		} else {
			log.Printf("Failed to unmarshal reviewer output: %v", err)
		}
	}
}

func createPlannerAgent() interfaces.Agent {
	// Create planner agent with structured output
	responseFormat := structuredoutput.NewResponseFormat(TerraformPlan{})
	log.Printf("Planner agent response format: %+v", responseFormat)

	planner, err := agent.NewAgent(
		agent.WithSystemPrompt(`You are a Terraform module planner. Your task is to analyze the requirements and create a plan for a Terraform module.
		You should consider:
		- Required resources
		- Variables needed
		- Outputs to be defined
		- Best practices for Terraform modules

		IMPORTANT: You MUST return a valid JSON object. DO NOT return any other format.
		DO NOT include any markdown, headers, or explanatory text.
		ONLY return the JSON object as specified below.

		Your response MUST follow this EXACT structure:
		{
			"module_name": string,
			"resources": [string],
			"variables": [string],
			"outputs": [string],
		}

		Where:
		- module_name: A descriptive name for the module
		- resources: List of resources that need to be created
		- variables: List of variables needed for the module
		- outputs: List of outputs to be defined
		Example of valid response:
		{
			"module_name": "s3_bucket_module",
			"resources": ["aws_s3_bucket", "aws_s3_bucket_versioning", "aws_s3_bucket_server_side_encryption_configuration"],
			"variables": ["bucket_name", "environment", "project"],
			"outputs": ["bucket_id", "bucket_arn"],
		}`),
		agent.WithResponseFormat(*responseFormat),
		agent.WithTools(&tools.TerraformValidateTool{}),
		agent.WithLLM(openaiClient),
	)
	if err != nil {
		log.Fatalf("Failed to create planner agent: %v", err)
	}
	return planner
}

func createGeneratorAgent(fileWriter *tools.FileWriterTool) interfaces.Agent {
	// Create generator agent with structured output
	generator, err := agent.NewAgent(
		agent.WithSystemPrompt(`You are a Terraform file generator. Your task is to create Terraform files based on the plan.

		IMPORTANT: You MUST follow these steps in order:
		1. Generate the Terraform files as specified below
		2. For EACH file, use the write_terraform_files tool to write it to disk
		3. Only after writing all files, proceed with the next step

		You MUST return exactly four files in your response:
		1. A file named "main.tf" containing ONLY resource definitions
		2. A file named "variables.tf" containing ONLY variable definitions
		3. A file named "outputs.tf" containing ONLY output definitions
		4. A file named "versions.tf" containing provider and version configurations

		DO NOT combine the files. DO NOT include outputs or variables in main.tf.
		Each file MUST be a separate entry in the files array.

		Follow these guidelines for each file:
		- main.tf:
		  * Include only resource definitions
		  * Use proper resource naming
		  * Follow security best practices
		  * Add descriptive comments

		- variables.tf:
		  * Define all variables used in main.tf
		  * Include type constraints
		  * Add descriptions
		  * Set default values where appropriate
		  * Add validation rules

		- outputs.tf:
		  * Define outputs for important resource attributes
		  * Add descriptions
		  * Include sensitive flag where needed

		- versions.tf:
		  * Specify required provider versions
		  * Configure AWS provider settings
		  * Set minimum required Terraform version

		If you receive review comments and suggestions, you MUST:
		1. Address all issues mentioned in the review
		2. Implement the suggested improvements
		3. Keep the same file structure but update the content
		4. Ensure the changes maintain compatibility with existing resources

		Your response MUST follow this EXACT structure:
		{
			"files": [
				{
					"name": "main.tf",
					"type": "main.tf",
					"content": "# Resource definitions only..."
				},
				{
					"name": "variables.tf",
					"type": "variables.tf",
					"content": "# Variable definitions only..."
				},
				{
					"name": "outputs.tf",
					"type": "outputs.tf",
					"content": "# Output definitions only..."
				},
				{
					"name": "versions.tf",
					"type": "versions.tf",
					"content": "# Provider and version configurations..."
				}
			]
		}

		After generating the files, you MUST use the write_terraform_files tool to write EACH file to disk.
		For each file, make a separate tool call following this EXACT format:

		For versions.tf:
		write_terraform_files({"name":"versions.tf","type":"versions.tf","content":"terraform {\n  required_providers {\n    aws = {\n      source  = \"hashicorp/aws\"\n      version = \"~> 3.0\"\n    }\n  }\n  required_version = \">= 0.12\"\n}"})

		For variables.tf:
		write_terraform_files({"name":"variables.tf","type":"variables.tf","content":"variable \"bucket_name\" {\n  description = \"The name of the S3 bucket\"\n  type        = string\n}"})

		For main.tf:
		write_terraform_files({"name":"main.tf","type":"main.tf","content":"resource \"aws_s3_bucket\" \"example\" {\n  bucket = var.bucket_name\n  acl    = \"private\"\n}"})

		For outputs.tf:
		write_terraform_files({"name":"outputs.tf","type":"outputs.tf","content":"output \"bucket_arn\" {\n  value       = aws_s3_bucket.example.arn\n  description = \"The ARN of the S3 bucket\"\n}"})

		IMPORTANT: The tool call format MUST be exactly as shown above. DO NOT:
		- Add extra quotes or escape characters
		- Include any additional fields
		- Modify the structure of the tool call

		Make sure to call the tool for each file separately, in this order:
		1. versions.tf (first, as it sets up providers)
		2. variables.tf (second, as other files depend on variables)
		3. main.tf (third, as it uses the variables)
		4. outputs.tf (last, as it depends on resources in main.tf)`),
		agent.WithResponseFormat(*structuredoutput.NewResponseFormat(FileGenerationPlan{})),
		agent.WithTools(fileWriter),
		agent.WithRequirePlanApproval(false),
		agent.WithLLM(openaiClient),
	)
	if err != nil {
		log.Fatalf("Failed to create generator agent: %v", err)
	}
	return generator
}

func createReviewerAgent(terraformValidate *tools.TerraformValidateTool) interfaces.Agent {
	// Create reviewer agent with structured output
	responseFormat := structuredoutput.NewResponseFormat(ReviewResult{})
	log.Printf("Reviewer agent response format: %+v", responseFormat)

	reviewer, err := agent.NewAgent(
		agent.WithSystemPrompt(`You are a Terraform file reviewer. Your task is to review the generated files and ensure they meet quality standards.

		IMPORTANT: You MUST return a valid JSON object. DO NOT return any other format.
		DO NOT include any markdown, headers, or explanatory text.
		ONLY return the JSON object as specified below.

		You MUST follow these steps:
		1. First, use the terraform_validate tool to validate the configuration files:
		   terraform_validate({"directory": "."})

		2. Then, check for:
		   - Correct syntax
		   - Best practices
		   - Security considerations
		   - Documentation quality
		   - Resource naming conventions

		3. Your response MUST be a valid JSON object with this EXACT structure:
		{
			"approved": boolean,
			"comments": [string],
			"suggestions": [string],
			"validation_results": string
		}

		Where:
		- approved: true only if both validation passes and no major improvements are needed
		- comments: list of issues found in the code
		- suggestions: list of suggested improvements
		- validation_results: the output from terraform_validate

		Example of valid response:
		{
			"approved": false,
			"comments": ["Missing required tags", "Incorrect resource naming"],
			"suggestions": ["Add environment tag", "Use consistent naming convention"],
			"validation_results": "Success! The configuration is valid."
		}

		If terraform_validate fails, set approved to false and include the error in comments.
		If terraform_validate succeeds but there are improvements needed, set approved to false and include suggestions.
		Only set approved to true if both validation passes and no major improvements are needed.`),
		agent.WithResponseFormat(*responseFormat),
		agent.WithTools(terraformValidate),
		agent.WithRequirePlanApproval(false),
		agent.WithLLM(openaiClient),
	)
	if err != nil {
		log.Fatalf("Failed to create reviewer agent: %v", err)
	}
	return reviewer
}
