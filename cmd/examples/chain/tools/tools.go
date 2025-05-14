package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

type FileWriteResult struct {
	Success bool   `json:"success" description:"Whether the file was written successfully"`
	Path    string `json:"path" description:"Path where the file was written"`
	Error   string `json:"error,omitempty" description:"Error message if any"`
}

type FileWriterTool struct {
	TempDir string
}

func (t *FileWriterTool) Name() string {
	return "write_terraform_files"
}

func (t *FileWriterTool) Description() string {
	return "Write Terraform files to a temporary directory"
}

func (t *FileWriterTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"name": {
			Type:        "string",
			Description: "Name of the file to write",
			Required:    true,
		},
		"type": {
			Type:        "string",
			Description: "Type of file (main.tf, variables.tf, etc)",
			Required:    true,
		},
		"content": {
			Type:        "string",
			Description: "Content of the file to write",
			Required:    true,
		},
	}
}

func (t *FileWriterTool) Execute(ctx context.Context, args string) (string, error) {
	log.Printf("Executing write_terraform_files with args: %s", args)

	var input struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("invalid input format: %w", err)
	}

	if t.TempDir == "" {
		tempDir, err := os.MkdirTemp("", "tf-files-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temp directory: %w", err)
		}
		t.TempDir = tempDir
		log.Printf("Created temporary directory: %s", t.TempDir)
	}

	filePath := filepath.Join(t.TempDir, input.Name)
	log.Printf("Writing file: %s", filePath)

	if err := os.WriteFile(filePath, []byte(input.Content), 0644); err != nil {
		result := FileWriteResult{
			Success: false,
			Path:    filePath,
			Error:   err.Error(),
		}
		resultJSON, _ := json.Marshal(result)
		return string(resultJSON), fmt.Errorf("failed to write file: %w", err)
	}

	result := FileWriteResult{
		Success: true,
		Path:    filePath,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}

func (t *FileWriterTool) Run(ctx context.Context, args string) (string, error) {
	return t.Execute(ctx, args)
}

type TerraformValidateTool struct {
	FileWriter *FileWriterTool
}

func (t *TerraformValidateTool) Name() string {
	return "terraform_validate"
}

func (t *TerraformValidateTool) Description() string {
	return "Validate Terraform configuration files"
}

func (t *TerraformValidateTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"directory": {
			Type:        "string",
			Description: "Directory containing Terraform files to validate",
			Required:    true,
		},
	}
}

func (t *TerraformValidateTool) Execute(ctx context.Context, args string) (string, error) {
	log.Printf("Executing terraform validate with args: %s", args)

	validateDir := t.FileWriter.TempDir
	if validateDir == "" {
		return "", fmt.Errorf("no temporary directory available for validation")
	}

	log.Printf("Initializing Terraform in directory: %s", validateDir)
	initCmd := exec.CommandContext(ctx, "terraform", "init")
	initCmd.Dir = validateDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		log.Printf("Terraform init failed: %v\nOutput: %s", err, string(output))
		return string(output), fmt.Errorf("terraform init failed: %w", err)
	}

	log.Printf("Running terraform validate in directory: %s", validateDir)
	validateCmd := exec.CommandContext(ctx, "terraform", "validate")
	validateCmd.Dir = validateDir
	output, err := validateCmd.CombinedOutput()
	if err != nil {
		log.Printf("Terraform validate failed: %v\nOutput: %s", err, string(output))
		return string(output), fmt.Errorf("terraform validate failed: %w", err)
	}

	log.Printf("Terraform validate completed successfully")
	return string(output), nil
}

func (t *TerraformValidateTool) Run(ctx context.Context, args string) (string, error) {
	return t.Execute(ctx, args)
}
