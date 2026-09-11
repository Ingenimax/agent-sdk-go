package openai

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	openaiapi "github.com/openai/openai-go/v2"
)

// UploadUserDataFile uploads a local file for later use as a model input and
// returns the OpenAI file ID.
func (c *OpenAIClient) UploadUserDataFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return c.UploadUserData(ctx, file, filepath.Base(path), "")
}

// UploadUserData uploads file content for later use as a model input and returns
// the OpenAI file ID.
func (c *OpenAIClient) UploadUserData(ctx context.Context, reader io.Reader, filename, contentType string) (string, error) {
	file, err := c.Client.Files.New(ctx, openaiapi.FileNewParams{
		File:    openaiapi.File(reader, filename, contentType),
		Purpose: openaiapi.FilePurposeUserData,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}

	return file.ID, nil
}
