# Advanced Agent Example

This example demonstrates a more comprehensive implementation of an AI agent using the Agent SDK with the execution plan functionality, logging, memory, and tool integration. It provides a command-line interface for interacting with an agent that can perform complex tasks with user approval.

## Overview

The Advanced Agent example showcases several key capabilities:

1. **Execution Plan Workflow**: Creates, displays, and executes plans with user approval
2. **Conversation Memory**: Maintains conversation history for context
3. **Logging**: Detailed logging of agent operations
4. **Web Search Tool**: Integration with Google Search (when configured)
5. **Configuration**: Uses environment variables for configuration
6. **Command-Line Interface**: Interactive CLI for agent interaction

## Features

### Execution Plan Management

The Advanced Agent includes a robust task management system that allows you to:

- List all execution plans with `plans` command
- Approve plans with `approve <task-id>` command
- Modify plans with `modify <task-id> <instructions>` command
- See detailed plan information including steps, tools, and parameters

### Conversation Flow

The agent uses a structured conversation loop that:
1. Processes user input
2. Detects special commands (exit, plans, approve, modify)
3. Generates plans for complex tasks
4. Displays plans with rich formatting
5. Executes approved plans and displays results

### Performance Tracking

The example includes performance tracking that logs:
- Processing time for queries
- Task approvals and modifications
- Error handling with detailed logging

## Implementation Details

The Advanced Agent example is implemented with these key components:

- **Agent Configuration**: Creates an agent with LLM, memory, tools, and execution plan approval
- **Tool Registry**: Configures available tools based on environment variables
- **Conversation Loop**: Handles user input and special commands
- **Plan Management**: Pretty-prints plans and manages plan states
- **Logging**: Structured logging for debugging and monitoring

## Running the Example

To run this example:

```bash
go run cmd/advanced/main.go
```

### Prerequisites

1. Set up the required environment variables (or create a `.env` file):
   ```
   OPENAI_API_KEY=your-api-key
   OPENAI_MODEL=gpt-4-turbo
   LOG_LEVEL=info
   GOOGLE_API_KEY=your-google-api-key (optional for web search)
   GOOGLE_SEARCH_ENGINE_ID=your-search-engine-id (optional for web search)
   ```

2. Install dependencies:
   ```
   go mod download
   ```

## Example Session

Here's an example of how a session with the Advanced Agent might flow:

```
Welcome to the Advanced Agent CLI Tool!
Type your questions or commands. The agent will create an execution plan for complex tasks.
Special commands:
  'exit' - Exit the application
  'plans' - List all execution plans
  'approve <task-id>' - Approve an execution plan
  'modify <task-id> <instructions>' - Modify an execution plan
-----------------------------------------

You: Can you help me deploy a web application on AWS?

Agent: I'll help you deploy a web application on AWS. Let me create an execution plan for that.

=== Execution Plan ===
Task ID: abc123def456
Description: Deploy a web application on AWS with load balancer and security
Status: pending_approval

Steps:
1. Configure AWS credentials and region
   Tool: aws_configure
   Parameters:
   {
     "region": "us-west-2",
     "profile": "default"
   }

2. Create infrastructure with CloudFormation
   Tool: aws_deploy
   Parameters:
   {
     "template": "web-app-template.yaml",
     "stack_name": "web-application-stack"
   }

To approve this plan, type: approve abc123def456
To modify this plan, type: modify abc123def456 <your instructions>
======================

You: modify abc123def456 Please use us-east-1 region instead and add a step for setting up CloudFront

Agent: I've updated the execution plan based on your feedback:

=== Execution Plan ===
Task ID: abc123def456
Description: Deploy a web application on AWS with load balancer, security, and CloudFront
Status: pending_approval

Steps:
1. Configure AWS credentials and region
   Tool: aws_configure
   Parameters:
   {
     "region": "us-east-1",
     "profile": "default"
   }

2. Create infrastructure with CloudFormation
   Tool: aws_deploy
   Parameters:
   {
     "template": "web-app-template.yaml",
     "stack_name": "web-application-stack"
   }

3. Set up CloudFront distribution
   Tool: aws_cloudfront
   Parameters:
   {
     "origin_domain": "${web-application-stack.LoadBalancerDNS}"
   }

To approve this plan, type: approve abc123def456
To modify this plan, type: modify abc123def456 <your instructions>
======================

You: approve abc123def456

Agent: Executing the approved plan...

Step 1 (Configure AWS credentials and region): Successfully configured AWS with region us-east-1

Step 2 (Create infrastructure with CloudFormation): Successfully deployed stack web-application-stack. Load balancer created at web-app-lb-123456789.us-east-1.elb.amazonaws.com

Step 3 (Set up CloudFront distribution): Successfully created CloudFront distribution. Domain: d1a2b3c4d5e6f7.cloudfront.net

Web application successfully deployed! You can access it at:
https://d1a2b3c4d5e6f7.cloudfront.net
```

## Customization

You can extend this example by:

1. Adding more tools to the tool registry
2. Modifying the conversation loop to handle additional commands
3. Implementing more sophisticated memory or logging solutions
4. Adding authentication or user management
5. Creating a web-based interface instead of CLI

## Integration with Your Application

To integrate this functionality into your application:

1. Use the agent creation pattern from `main.go`
2. Adapt the conversation loop to your UI (web, mobile, etc.)
3. Implement custom tools for your specific use case
4. Extend the execution plan visualization to match your application's UI 