package calculator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// Calculator implements a simple calculator tool
type Calculator struct {}

// Input represents the input for the calculator tool
type Input struct {
	Expression string `json:"expression"`
}

// New creates a new calculator tool
func New() *Calculator {
	return &Calculator{}
}

// Name implements interfaces.Tool.Name
func (c *Calculator) Name() string {
	return "calculator"
}

// DisplayName implements interfaces.ToolWithDisplayName.DisplayName
func (c *Calculator) DisplayName() string {
	return "Calculator"
}

// Description implements interfaces.Tool.Description
func (c *Calculator) Description() string {
	return "Perform mathematical calculations (add, subtract, multiply, divide, exponents)"
}

// Internal implements interfaces.InternalTool.Internal
func (c *Calculator) Internal() bool {
	return false
}

// Parameters implements interfaces.Tool.Parameters
func (c *Calculator) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"expression": {
			Type:        "string",
			Description: "The mathematical expression to evaluate (e.g., '2 + 2', '10 * 5', '7 / 3')",
			Required:    true,
		},
	}
}

// Run implements interfaces.Tool.Run
func (c *Calculator) Run(ctx context.Context, input string) (string, error) {
	// Simplify the input and evaluate
	input = strings.TrimSpace(input)
	// Handle simple operations with basic parsing
	return c.evaluateExpression(input)
}

// Execute implements interfaces.Tool.Execute
func (c *Calculator) Execute(ctx context.Context, args string) (string, error) {
	var input Input
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	return c.evaluateExpression(input.Expression)
}

// evaluateExpression evaluates a simple mathematical expression
func (c *Calculator) evaluateExpression(expr string) (string, error) {
	// Remove all spaces
	expr = strings.ReplaceAll(expr, " ", "")

	// Try to handle common operations
	if strings.Contains(expr, "+") {
		return c.handleAddition(expr)
	} else if strings.Contains(expr, "-") {
		return c.handleSubtraction(expr)
	} else if strings.Contains(expr, "*") {
		return c.handleMultiplication(expr)
	} else if strings.Contains(expr, "/") {
		return c.handleDivision(expr)
	} else if strings.Contains(expr, "^") {
		return c.handleExponent(expr)
	}

	// Try to parse as a single number
	if num, err := strconv.ParseFloat(expr, 64); err == nil {
		return fmt.Sprintf("%g", num), nil
	}

	return "", fmt.Errorf("unsupported expression: %s", expr)
}

// handleAddition handles addition expressions
func (c *Calculator) handleAddition(expr string) (string, error) {
	parts := strings.Split(expr, "+")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid addition format: %s", expr)
	}

	a, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "", fmt.Errorf("invalid first operand: %s", parts[0])
	}

	b, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid second operand: %s", parts[1])
	}

	result := a + b
	return fmt.Sprintf("%g", result), nil
}

// handleSubtraction handles subtraction expressions
func (c *Calculator) handleSubtraction(expr string) (string, error) {
	parts := strings.Split(expr, "-")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid subtraction format: %s", expr)
	}

	// Handle negative first number
	if parts[0] == "" {
		if len(parts) < 3 {
			return "", fmt.Errorf("invalid subtraction format with negative: %s", expr)
		}
		a, err := strconv.ParseFloat("-"+parts[1], 64)
		if err != nil {
			return "", fmt.Errorf("invalid first operand: -%s", parts[1])
		}

		b, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return "", fmt.Errorf("invalid second operand: %s", parts[2])
		}

		result := a - b
		return fmt.Sprintf("%g", result), nil
	}

	a, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "", fmt.Errorf("invalid first operand: %s", parts[0])
	}

	b, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid second operand: %s", parts[1])
	}

	result := a - b
	return fmt.Sprintf("%g", result), nil
}

// handleMultiplication handles multiplication expressions
func (c *Calculator) handleMultiplication(expr string) (string, error) {
	parts := strings.Split(expr, "*")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid multiplication format: %s", expr)
	}

	a, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "", fmt.Errorf("invalid first operand: %s", parts[0])
	}

	b, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid second operand: %s", parts[1])
	}

	result := a * b
	return fmt.Sprintf("%g", result), nil
}

// handleDivision handles division expressions
func (c *Calculator) handleDivision(expr string) (string, error) {
	parts := strings.Split(expr, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid division format: %s", expr)
	}

	a, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "", fmt.Errorf("invalid first operand: %s", parts[0])
	}

	b, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid second operand: %s", parts[1])
	}

	if b == 0 {
		return "", fmt.Errorf("division by zero")
	}

	result := a / b
	return fmt.Sprintf("%g", result), nil
}

// handleExponent handles exponent expressions
func (c *Calculator) handleExponent(expr string) (string, error) {
	parts := strings.Split(expr, "^")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid exponent format: %s", expr)
	}

	base, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return "", fmt.Errorf("invalid base: %s", parts[0])
	}

	exp, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", fmt.Errorf("invalid exponent: %s", parts[1])
	}

	result := math.Pow(base, exp)
	return fmt.Sprintf("%g", result), nil
}
