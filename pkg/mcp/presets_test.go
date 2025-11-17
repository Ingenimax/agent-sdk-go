package mcp

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPreset(t *testing.T) {
	t.Run("valid preset without required env", func(t *testing.T) {
		config, err := GetPreset("filesystem")

		assert.NoError(t, err)
		assert.Equal(t, "filesystem", config.Name)
		assert.Equal(t, "stdio", config.Type)
		assert.Equal(t, "npx", config.Command)
		assert.Contains(t, config.Args, "@modelcontextprotocol/server-filesystem")
		assert.Empty(t, config.Env)
	})

	t.Run("valid preset with required env set", func(t *testing.T) {
		// Set required environment variable
		originalToken := os.Getenv("GITHUB_TOKEN")
		defer func() {
			if originalToken == "" {
				os.Unsetenv("GITHUB_TOKEN")
			} else {
				os.Setenv("GITHUB_TOKEN", originalToken)
			}
		}()
		os.Setenv("GITHUB_TOKEN", "test-token")

		config, err := GetPreset("github")

		assert.NoError(t, err)
		assert.Equal(t, "github", config.Name)
		assert.Equal(t, "stdio", config.Type)
		assert.Equal(t, "npx", config.Command)
		assert.Contains(t, config.Args, "@modelcontextprotocol/server-github")
		assert.Contains(t, config.Env, "GITHUB_TOKEN=test-token")
	})

	t.Run("preset with required env not set", func(t *testing.T) {
		// Ensure environment variable is not set
		originalToken := os.Getenv("GITHUB_TOKEN")
		defer func() {
			if originalToken == "" {
				os.Unsetenv("GITHUB_TOKEN")
			} else {
				os.Setenv("GITHUB_TOKEN", originalToken)
			}
		}()
		os.Unsetenv("GITHUB_TOKEN")

		config, err := GetPreset("github")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires environment variable GITHUB_TOKEN")
		assert.Equal(t, LazyMCPServerConfig{}, config)
	})

	t.Run("preset with multiple required env", func(t *testing.T) {
		// Set up environment variables for Slack preset
		originalBotToken := os.Getenv("SLACK_BOT_TOKEN")
		originalTeamID := os.Getenv("SLACK_TEAM_ID")
		defer func() {
			if originalBotToken == "" {
				os.Unsetenv("SLACK_BOT_TOKEN")
			} else {
				os.Setenv("SLACK_BOT_TOKEN", originalBotToken)
			}
			if originalTeamID == "" {
				os.Unsetenv("SLACK_TEAM_ID")
			} else {
				os.Setenv("SLACK_TEAM_ID", originalTeamID)
			}
		}()
		os.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
		os.Setenv("SLACK_TEAM_ID", "T123456")

		config, err := GetPreset("slack")

		assert.NoError(t, err)
		assert.Equal(t, "slack", config.Name)
		assert.Contains(t, config.Env, "SLACK_BOT_TOKEN=xoxb-test")
		assert.Contains(t, config.Env, "SLACK_TEAM_ID=T123456")
	})

	t.Run("preset with one missing env from multiple", func(t *testing.T) {
		// Set only one of the required environment variables
		originalBotToken := os.Getenv("SLACK_BOT_TOKEN")
		originalTeamID := os.Getenv("SLACK_TEAM_ID")
		defer func() {
			if originalBotToken == "" {
				os.Unsetenv("SLACK_BOT_TOKEN")
			} else {
				os.Setenv("SLACK_BOT_TOKEN", originalBotToken)
			}
			if originalTeamID == "" {
				os.Unsetenv("SLACK_TEAM_ID")
			} else {
				os.Setenv("SLACK_TEAM_ID", originalTeamID)
			}
		}()
		os.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
		os.Unsetenv("SLACK_TEAM_ID")

		config, err := GetPreset("slack")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires environment variable SLACK_TEAM_ID")
		assert.Equal(t, LazyMCPServerConfig{}, config)
	})

	t.Run("nonexistent preset", func(t *testing.T) {
		config, err := GetPreset("nonexistent")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), `preset "nonexistent" not found`)
		assert.Equal(t, LazyMCPServerConfig{}, config)
	})
}

func TestListPresets(t *testing.T) {
	presets := ListPresets()

	// Should include known presets
	expectedPresets := []string{
		"filesystem",
		"github",
		"git",
		"postgres",
		"slack",
		"gdrive",
		"puppeteer",
		"memory",
		"fetch",
		"brave-search",
		"time",
		"sqlite",
		"docker",
		"kubectl",
		"aws",
	}

	assert.Len(t, presets, len(expectedPresets))

	sort.Strings(presets)
	sort.Strings(expectedPresets)
	assert.Equal(t, expectedPresets, presets)
}

func TestGetPresetInfo(t *testing.T) {
	tests := []struct {
		name           string
		presetName     string
		expectError    bool
		expectedFields []string
	}{
		{
			name:       "filesystem preset",
			presetName: "filesystem",
			expectedFields: []string{
				"Name: filesystem",
				"Description: MCP server for file system operations",
				"Type: stdio",
				"Command: npx",
				"Args:",
			},
		},
		{
			name:       "github preset with required env",
			presetName: "github",
			expectedFields: []string{
				"Name: github",
				"Description: MCP server for GitHub operations",
				"Type: stdio",
				"Required Environment Variables:",
				"GITHUB_TOKEN",
			},
		},
		{
			name:       "postgres preset",
			presetName: "postgres",
			expectedFields: []string{
				"Name: postgres",
				"Type: stdio",
				"DATABASE_URL",
			},
		},
		{
			name:        "nonexistent preset",
			presetName:  "nonexistent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := GetPresetInfo(tt.presetName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not found")
				assert.Empty(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, info)

				for _, expectedField := range tt.expectedFields {
					assert.Contains(t, info, expectedField,
						"Info should contain %q. Got: %s", expectedField, info)
				}
			}
		})
	}
}

func TestGetPresetInfo_AllKnownPresets(t *testing.T) {
	allPresets := ListPresets()

	for _, presetName := range allPresets {
		t.Run("preset_"+presetName, func(t *testing.T) {
			info, err := GetPresetInfo(presetName)

			assert.NoError(t, err)
			assert.NotEmpty(t, info)
			assert.Contains(t, info, "Name: "+presetName)
			assert.Contains(t, info, "Description:")
			assert.Contains(t, info, "Type:")
		})
	}
}

func TestPresetStructure(t *testing.T) {
	// Test that all presets in the map have the required basic fields
	for name, preset := range presets {
		t.Run("preset_structure_"+name, func(t *testing.T) {
			assert.NotEmpty(t, preset.Name, "Preset %s should have a name", name)
			assert.Equal(t, name, preset.Name, "Preset key should match name")
			assert.NotEmpty(t, preset.Description, "Preset %s should have a description", name)
			assert.Contains(t, []string{"stdio", "http"}, preset.Type,
				"Preset %s should have valid type", name)

			if preset.Type == "stdio" {
				assert.NotEmpty(t, preset.Command,
					"Stdio preset %s should have a command", name)
			} else if preset.Type == "http" {
				assert.NotEmpty(t, preset.URL,
					"HTTP preset %s should have a URL", name)
			}
		})
	}
}

func TestSpecificPresets(t *testing.T) {
	t.Run("filesystem preset details", func(t *testing.T) {
		preset, exists := presets["filesystem"]
		require.True(t, exists)

		assert.Equal(t, "filesystem", preset.Name)
		assert.Equal(t, "stdio", preset.Type)
		assert.Equal(t, "npx", preset.Command)
		assert.Contains(t, preset.Args, "-y")
		assert.Contains(t, preset.Args, "@modelcontextprotocol/server-filesystem")
		assert.Empty(t, preset.RequiredEnv)
	})

	t.Run("github preset details", func(t *testing.T) {
		preset, exists := presets["github"]
		require.True(t, exists)

		assert.Equal(t, "github", preset.Name)
		assert.Equal(t, "stdio", preset.Type)
		assert.Contains(t, preset.RequiredEnv, "GITHUB_TOKEN")
		assert.Contains(t, preset.Args, "@modelcontextprotocol/server-github")
	})

	t.Run("aws preset details", func(t *testing.T) {
		preset, exists := presets["aws"]
		require.True(t, exists)

		assert.Equal(t, "aws", preset.Name)
		assert.Equal(t, "stdio", preset.Type)
		assert.Equal(t, "docker", preset.Command)
		assert.Contains(t, preset.Args, "run")
		assert.Contains(t, preset.Args, "--rm")
		assert.Contains(t, preset.RequiredEnv, "AWS_REGION")
		assert.Contains(t, preset.RequiredEnv, "AWS_ACCESS_KEY_ID")
		assert.Contains(t, preset.RequiredEnv, "AWS_SECRET_ACCESS_KEY")
	})

	t.Run("docker preset details", func(t *testing.T) {
		preset, exists := presets["docker"]
		require.True(t, exists)

		assert.Equal(t, "docker", preset.Name)
		assert.Equal(t, "stdio", preset.Type)
		assert.Equal(t, "docker", preset.Command)
		assert.Contains(t, preset.Args, "/var/run/docker.sock:/var/run/docker.sock")
		assert.Empty(t, preset.RequiredEnv)
	})

	t.Run("sqlite preset with args", func(t *testing.T) {
		preset, exists := presets["sqlite"]
		require.True(t, exists)

		assert.Equal(t, "sqlite", preset.Name)
		assert.Contains(t, preset.Args, "@modelcontextprotocol/server-sqlite")
		assert.Contains(t, preset.Args, "path/to/database.db")
	})
}

// Test environment variable handling with complex scenarios
func TestGetPreset_EnvironmentVariables(t *testing.T) {
	t.Run("preset with existing env vars", func(t *testing.T) {
		// Mock preset with custom environment
		originalPresets := presets
		defer func() { presets = originalPresets }()

		presets["test-env"] = PresetServer{
			Name:        "test-env",
			Type:        "stdio",
			Command:     "test",
			Env:         []string{"CUSTOM_VAR=custom_value"},
			RequiredEnv: []string{"REQUIRED_VAR"},
		}

		// Set required env
		originalVar := os.Getenv("REQUIRED_VAR")
		defer func() {
			if originalVar == "" {
				os.Unsetenv("REQUIRED_VAR")
			} else {
				os.Setenv("REQUIRED_VAR", originalVar)
			}
		}()
		os.Setenv("REQUIRED_VAR", "required_value")

		config, err := GetPreset("test-env")

		assert.NoError(t, err)
		assert.Contains(t, config.Env, "CUSTOM_VAR=custom_value")
		assert.Contains(t, config.Env, "REQUIRED_VAR=required_value")
	})

	t.Run("empty environment variable", func(t *testing.T) {
		// Test when required env var exists but is empty
		originalToken := os.Getenv("GITHUB_TOKEN")
		defer func() {
			if originalToken == "" {
				os.Unsetenv("GITHUB_TOKEN")
			} else {
				os.Setenv("GITHUB_TOKEN", originalToken)
			}
		}()
		os.Setenv("GITHUB_TOKEN", "")

		config, err := GetPreset("github")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires environment variable GITHUB_TOKEN")
		assert.Equal(t, LazyMCPServerConfig{}, config)
	})
}

// Test edge cases and error conditions
func TestGetPreset_EdgeCases(t *testing.T) {
	t.Run("empty preset name", func(t *testing.T) {
		config, err := GetPreset("")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), `preset "" not found`)
		assert.Equal(t, LazyMCPServerConfig{}, config)
	})

	t.Run("case sensitive preset names", func(t *testing.T) {
		config, err := GetPreset("FILESYSTEM")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), `preset "FILESYSTEM" not found`)
		assert.Equal(t, LazyMCPServerConfig{}, config)
	})
}

func TestGetPresetInfo_EdgeCases(t *testing.T) {
	t.Run("empty preset name", func(t *testing.T) {
		info, err := GetPresetInfo("")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), `preset "" not found`)
		assert.Empty(t, info)
	})

	t.Run("case sensitive preset names", func(t *testing.T) {
		info, err := GetPresetInfo("GIT")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), `preset "GIT" not found`)
		assert.Empty(t, info)
	})
}

// Test that preset info format is consistent
func TestGetPresetInfo_Format(t *testing.T) {
	tests := []struct {
		name             string
		presetName       string
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			name:       "stdio preset format",
			presetName: "git",
			shouldContain: []string{
				"Name: git",
				"Description:",
				"Type: stdio",
				"Command:",
			},
			shouldNotContain: []string{
				"URL:",
			},
		},
		{
			name:       "preset with no args",
			presetName: "kubectl",
			shouldContain: []string{
				"Name: kubectl",
				"Type: stdio",
				"Command: kubectl-mcp",
			},
		},
		{
			name:       "preset with required env",
			presetName: "postgres",
			shouldContain: []string{
				"Required Environment Variables:",
				"DATABASE_URL",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := GetPresetInfo(tt.presetName)
			require.NoError(t, err)

			for _, shouldContain := range tt.shouldContain {
				assert.Contains(t, info, shouldContain)
			}

			for _, shouldNotContain := range tt.shouldNotContain {
				assert.NotContains(t, info, shouldNotContain)
			}
		})
	}
}

// Performance tests
func BenchmarkGetPreset(b *testing.B) {
	presetNames := []string{"filesystem", "github", "git", "postgres"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetPreset(presetNames[i%len(presetNames)])
	}
}

func BenchmarkListPresets(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ListPresets()
	}
}

func BenchmarkGetPresetInfo(b *testing.B) {
	presetNames := []string{"filesystem", "github", "git"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetPresetInfo(presetNames[i%len(presetNames)])
	}
}

// Test concurrent access to presets
func TestPresetsThreadSafety(t *testing.T) {
	// Test concurrent reads of presets
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()

			presetNames := []string{"filesystem", "git", "memory"}
			for j := 0; j < 100; j++ {
				_, _ = GetPreset(presetNames[j%len(presetNames)])
				_ = ListPresets()
				_, _ = GetPresetInfo(presetNames[j%len(presetNames)])
			}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test that all NPX-based presets use consistent arguments
func TestNpxPresetsConsistency(t *testing.T) {
	npxPresets := []string{
		"filesystem", "github", "git", "postgres", "slack",
		"gdrive", "puppeteer", "memory", "fetch", "brave-search",
		"time", "sqlite",
	}

	for _, name := range npxPresets {
		t.Run("npx_preset_"+name, func(t *testing.T) {
			preset, exists := presets[name]
			require.True(t, exists)

			assert.Equal(t, "stdio", preset.Type)
			assert.Equal(t, "npx", preset.Command)
			assert.Contains(t, preset.Args, "-y")

			// Should have the package name in args
			found := false
			for _, arg := range preset.Args {
				if strings.Contains(arg, "@modelcontextprotocol/server-") {
					found = true
					break
				}
			}
			assert.True(t, found, "NPX preset should contain @modelcontextprotocol package")
		})
	}
}

// Test Docker-based presets
func TestDockerPresetsConsistency(t *testing.T) {
	dockerPresets := []string{"docker", "aws"}

	for _, name := range dockerPresets {
		t.Run("docker_preset_"+name, func(t *testing.T) {
			preset, exists := presets[name]
			require.True(t, exists)

			assert.Equal(t, "stdio", preset.Type)
			assert.Equal(t, "docker", preset.Command)
			assert.Contains(t, preset.Args, "run")
			assert.Contains(t, preset.Args, "--rm")
			assert.Contains(t, preset.Args, "-i")
		})
	}
}
