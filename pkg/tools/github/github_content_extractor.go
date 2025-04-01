package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

// GitHubContentExtractorTool is a tool for extracting content from GitHub repositories
type GitHubContentExtractorTool struct {
	client *github.Client
}

// NewGitHubContentExtractorTool creates a new instance of GitHubContentExtractorTool
func NewGitHubContentExtractorTool(token string) (*GitHubContentExtractorTool, error) {
	ctx := context.Background()
	var client *github.Client

	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)
		client = github.NewClient(tc)
	} else {
		client = github.NewClient(nil)
	}

	return &GitHubContentExtractorTool{
		client: client,
	}, nil
}

// Name returns the name of the tool
func (gct *GitHubContentExtractorTool) Name() string {
	return "github_content_extractor"
}

// Description returns the description of the tool
func (gct *GitHubContentExtractorTool) Description() string {
	return "Extracts content from GitHub repositories based on file patterns"
}

// Parameters returns the parameters that the tool accepts
func (gct *GitHubContentExtractorTool) Parameters() map[string]interfaces.ParameterSpec {
	return map[string]interfaces.ParameterSpec{
		"repository_url": {
			Type:        "string",
			Description: "The GitHub repository URL to analyze",
			Required:    true,
		},
		"file_patterns": {
			Type:        "array",
			Description: "List of file patterns to match (e.g., ['.json', '.yaml']). Use only the following extensions: .json, .yaml, .yml, .go, .py, .js, .ts, .java, .rb, .php, .cs, .cpp, .c, .h, .hpp, .swift, .kt, .scala, .rs, .dart, .lua, .proto, .thrift, .avro, .graphql, .sql, .tf, .tfvars, .hcl, .tfstate, .k8s.yaml, .k8s.yml",
			Required:    true,
			Items: &interfaces.ParameterSpec{
				Type: "string",
			},
		},
		"max_files": {
			Type:        "number",
			Description: "Maximum number of files to extract",
			Required:    false,
		},
		"max_depth": {
			Type:        "number",
			Description: "Maximum depth of directories to traverse",
			Required:    false,
		},
		"max_file_size": {
			Type:        "number",
			Description: "Maximum file size to extract (in bytes)",
			Required:    false,
		},
		"specific_files": {
			Type:        "array",
			Description: "List of specific files to extract",
			Required:    false,
			Items: &interfaces.ParameterSpec{
				Type: "string",
			},
		},
	}
}

// FileContent represents the structure of each file's content
type FileContent struct {
	FileName string `json:"file_name"`
	Path     string `json:"path"`
	Content  string `json:"content"`
}

// SearchParams represents the parameters for searching files
type SearchParams struct {
	RepositoryURL string   `json:"repository_url"`
	FilePatterns  []string `json:"file_patterns"`
	MaxFiles      int      `json:"max_files,omitempty"`
	MaxDepth      int      `json:"max_depth,omitempty"`
	MaxFileSize   int64    `json:"max_file_size,omitempty"`
	SpecificFiles []string `json:"specific_files,omitempty"`
}

// Run executes the tool with the given input
func (gct *GitHubContentExtractorTool) Run(ctx context.Context, input string) (string, error) {
	// Parse input as JSON
	var params SearchParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Extract repository information from input
	owner, repo, err := extractRepoInfo(params.RepositoryURL)
	if err != nil {
		return "", fmt.Errorf("failed to extract repository info: %w", err)
	}

	// Get repository contents
	contents, err := gct.getRepositoryContents(ctx, owner, repo)
	if err != nil {
		return "", fmt.Errorf("failed to get repository contents: %w", err)
	}

	// Find and extract matching files
	files, err := gct.findMatchingFiles(ctx, owner, repo, contents, params)
	if err != nil {
		return "", fmt.Errorf("failed to find matching files: %w", err)
	}

	// Convert to JSON
	result, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	return string(result), nil
}

// Execute implements the tool interface
func (gct *GitHubContentExtractorTool) Execute(ctx context.Context, args string) (string, error) {
	return gct.Run(ctx, args)
}

// findMatchingFiles finds all files matching the given patterns
func (gct *GitHubContentExtractorTool) findMatchingFiles(ctx context.Context, owner, repo string, contents []*github.RepositoryContent, params SearchParams) ([]FileContent, error) {
	var matchingFiles []FileContent

	// Helper function to check if a file matches specific files list
	matchesSpecificFile := func(path string) bool {
		if len(params.SpecificFiles) == 0 {
			return true
		}
		for _, specificFile := range params.SpecificFiles {
			if strings.HasSuffix(path, specificFile) {
				return true
			}
		}
		return false
	}

	// Helper function to process a directory
	var processDirectory func(path string, contents []*github.RepositoryContent, depth int) error
	processDirectory = func(path string, contents []*github.RepositoryContent, depth int) error {
		// Check depth limit
		if params.MaxDepth > 0 && depth > params.MaxDepth {
			return nil
		}

		// Check max files limit
		if params.MaxFiles > 0 && len(matchingFiles) >= params.MaxFiles {
			return fmt.Errorf("reached maximum number of files (%d)", params.MaxFiles)
		}

		for _, content := range contents {
			if content == nil || content.Type == nil {
				continue
			}

			// Handle directories
			if *content.Type == "dir" {
				// Get contents of the subdirectory
				fileContent, directoryContent, _, err := gct.client.Repositories.GetContents(ctx, owner, repo, *content.Path, nil)
				if err != nil {
					return fmt.Errorf("failed to get contents of directory %s: %w", *content.Path, err)
				}

				// Handle the response based on its type
				var contentsSlice []*github.RepositoryContent
				if fileContent != nil {
					// Single file/directory
					contentsSlice = []*github.RepositoryContent{fileContent}
				} else if directoryContent != nil {
					// Directory with multiple items
					contentsSlice = directoryContent
				}

				// Process subdirectory
				if err := processDirectory(*content.Path, contentsSlice, depth+1); err != nil {
					if strings.Contains(err.Error(), "reached maximum number of files") {
						return err
					}
					// Continue processing other directories even if one fails
				}
				continue
			}

			// Handle files
			if *content.Type == "file" {
				// Check if file matches specific files list
				if !matchesSpecificFile(*content.Path) {
					continue
				}

				// Check if file matches any of the patterns
				matches := false
				for _, pattern := range params.FilePatterns {
					if strings.HasSuffix(*content.Name, pattern) {
						matches = true
						break
					}
				}

				if matches {
					// Check file size limit
					if params.MaxFileSize > 0 && content.Size != nil && int64(*content.Size) > params.MaxFileSize {
						continue
					}

					fileContent, err := gct.getFileContent(ctx, owner, repo, *content.Path)
					if err != nil {
						return fmt.Errorf("failed to get content of file %s: %w", *content.Path, err)
					}

					// Extract directory path
					path := *content.Path
					fileName := *content.Name
					dirPath := path[:len(path)-len(fileName)]

					matchingFiles = append(matchingFiles, FileContent{
						FileName: fileName,
						Path:     dirPath,
						Content:  *fileContent,
					})

					// Check max files limit after adding a file
					if params.MaxFiles > 0 && len(matchingFiles) >= params.MaxFiles {
						return fmt.Errorf("reached maximum number of files (%d)", params.MaxFiles)
					}
				}
			}
		}
		return nil
	}

	// Start processing from root
	if err := processDirectory("", contents, 0); err != nil {
		if strings.Contains(err.Error(), "reached maximum number of files") {
			// This is not a real error, just indicates we hit the limit
			return matchingFiles, nil
		}
		return nil, err
	}

	return matchingFiles, nil
}

// getFileContent retrieves the content of a file
func (gct *GitHubContentExtractorTool) getFileContent(ctx context.Context, owner, repo, path string) (*string, error) {
	fileContent, _, _, err := gct.client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		return nil, err
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return nil, err
	}

	return &content, nil
}

// getRepositoryContents retrieves the contents of the repository
func (gct *GitHubContentExtractorTool) getRepositoryContents(ctx context.Context, owner, repo string) ([]*github.RepositoryContent, error) {
	_, contents, _, err := gct.client.Repositories.GetContents(ctx, owner, repo, "", nil)
	if err != nil {
		return nil, err
	}
	if contents == nil {
		return []*github.RepositoryContent{}, nil
	}
	return contents, nil
}

// extractRepoInfo extracts owner and repository name from GitHub URL
func extractRepoInfo(url string) (string, string, error) {
	parts := strings.Split(strings.TrimPrefix(url, "https://github.com/"), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid GitHub URL format %s", url)
	}
	return parts[0], parts[1], nil
}
