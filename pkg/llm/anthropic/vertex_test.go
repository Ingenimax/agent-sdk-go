package anthropic

import (
	"context"
	"testing"
	"time"
)

func TestVertexConfig_ParseRegions(t *testing.T) {
	tests := []struct {
		name            string
		regionInput     string
		expectedRegions []string
	}{
		{
			name:            "single region",
			regionInput:     "us-east5",
			expectedRegions: []string{"us-east5"},
		},
		{
			name:            "multiple regions without spaces",
			regionInput:     "asia-east1,us-east5,europe-west1",
			expectedRegions: []string{"asia-east1", "us-east5", "europe-west1"},
		},
		{
			name:            "multiple regions with spaces",
			regionInput:     "asia-east1 , us-east5 , europe-west1",
			expectedRegions: []string{"asia-east1", "us-east5", "europe-west1"},
		},
		{
			name:            "empty region",
			regionInput:     "",
			expectedRegions: []string{},
		},
		{
			name:            "region with trailing comma",
			regionInput:     "us-east5,europe-west1,",
			expectedRegions: []string{"us-east5", "europe-west1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc := &VertexConfig{
				Region: tt.regionInput,
			}
			vc.parseRegions()

			if len(vc.regions) != len(tt.expectedRegions) {
				t.Errorf("expected %d regions, got %d", len(tt.expectedRegions), len(vc.regions))
				return
			}

			for i, expected := range tt.expectedRegions {
				if vc.regions[i] != expected {
					t.Errorf("region[%d]: expected %s, got %s", i, expected, vc.regions[i])
				}
			}
		})
	}
}

func TestVertexConfig_GetCurrentRegion(t *testing.T) {
	tests := []struct {
		name           string
		regionInput    string
		expectedRegion string
	}{
		{
			name:           "single region",
			regionInput:    "us-east5",
			expectedRegion: "us-east5",
		},
		{
			name:           "multiple regions returns first",
			regionInput:    "asia-east1,us-east5,europe-west1",
			expectedRegion: "asia-east1",
		},
		{
			name:           "empty region returns original",
			regionInput:    "",
			expectedRegion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc := &VertexConfig{
				Region: tt.regionInput,
			}
			vc.parseRegions()

			currentRegion := vc.GetCurrentRegion()
			if currentRegion != tt.expectedRegion {
				t.Errorf("expected %s, got %s", tt.expectedRegion, currentRegion)
			}
		})
	}
}

func TestVertexConfig_RotateRegion(t *testing.T) {
	t.Run("single region does not rotate", func(t *testing.T) {
		vc := &VertexConfig{
			Region: "us-east5",
		}
		vc.parseRegions()

		initialRegion := vc.GetCurrentRegion()
		vc.RotateRegion()
		afterRotate := vc.GetCurrentRegion()

		if initialRegion != afterRotate {
			t.Errorf("single region should not rotate: expected %s, got %s", initialRegion, afterRotate)
		}
	})

	t.Run("multiple regions rotate in order", func(t *testing.T) {
		vc := &VertexConfig{
			Region: "asia-east1,us-east5,europe-west1",
		}
		vc.parseRegions()

		expectedOrder := []string{"asia-east1", "us-east5", "europe-west1"}

		for i := 0; i < len(expectedOrder)*2; i++ {
			currentRegion := vc.GetCurrentRegion()
			expectedRegion := expectedOrder[i%len(expectedOrder)]

			if currentRegion != expectedRegion {
				t.Errorf("rotation[%d]: expected %s, got %s", i, expectedRegion, currentRegion)
			}

			vc.RotateRegion()
		}
	})

	t.Run("rotation wraps around", func(t *testing.T) {
		vc := &VertexConfig{
			Region: "region-1,region-2,region-3",
		}
		vc.parseRegions()

		if vc.GetCurrentRegion() != "region-1" {
			t.Errorf("expected region-1, got %s", vc.GetCurrentRegion())
		}

		vc.RotateRegion()
		if vc.GetCurrentRegion() != "region-2" {
			t.Errorf("expected region-2, got %s", vc.GetCurrentRegion())
		}

		vc.RotateRegion()
		if vc.GetCurrentRegion() != "region-3" {
			t.Errorf("expected region-3, got %s", vc.GetCurrentRegion())
		}

		vc.RotateRegion()
		if vc.GetCurrentRegion() != "region-1" {
			t.Errorf("expected to wrap to region-1, got %s", vc.GetCurrentRegion())
		}
	})
}

func TestVertexConfig_GetBaseURL(t *testing.T) {
	t.Run("single region", func(t *testing.T) {
		vc := &VertexConfig{
			Enabled: true,
			Region:  "us-east5",
		}
		vc.parseRegions()

		baseURL := vc.GetBaseURL()
		expectedURL := "https://us-east5-aiplatform.googleapis.com"

		if baseURL != expectedURL {
			t.Errorf("expected %s, got %s", expectedURL, baseURL)
		}
	})

	t.Run("multiple regions uses current region", func(t *testing.T) {
		vc := &VertexConfig{
			Enabled: true,
			Region:  "asia-east1,us-east5",
		}
		vc.parseRegions()

		baseURL := vc.GetBaseURL()
		expectedURL := "https://asia-east1-aiplatform.googleapis.com"

		if baseURL != expectedURL {
			t.Errorf("expected %s, got %s", expectedURL, baseURL)
		}

		vc.RotateRegion()
		baseURL = vc.GetBaseURL()
		expectedURL = "https://us-east5-aiplatform.googleapis.com"

		if baseURL != expectedURL {
			t.Errorf("after rotation expected %s, got %s", expectedURL, baseURL)
		}
	})
}

func TestVertexRetryExecutor_Execute(t *testing.T) {
	t.Run("successful operation on first attempt", func(t *testing.T) {
		vc := &VertexConfig{
			Enabled: true,
			Region:  "us-east5,europe-west1",
		}
		vc.parseRegions()

		policy := &Policy{
			InitialInterval:    time.Millisecond * 10,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Second,
			MaximumAttempts:    3,
		}

		executor := NewVertexRetryExecutor(vc, policy)

		attempts := 0
		operation := func() error {
			attempts++
			return nil
		}

		ctx := context.Background()
		err := executor.Execute(ctx, operation)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}

		if vc.GetCurrentRegion() != "us-east5" {
			t.Errorf("expected region not to rotate on success, got %s", vc.GetCurrentRegion())
		}
	})

	t.Run("retries with region rotation", func(t *testing.T) {
		vc := &VertexConfig{
			Enabled: true,
			Region:  "region-1,region-2,region-3",
		}
		vc.parseRegions()

		policy := &Policy{
			InitialInterval:    time.Millisecond * 10,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Second,
			MaximumAttempts:    3,
		}

		executor := NewVertexRetryExecutor(vc, policy)

		attempts := 0
		regionsUsed := []string{}
		operation := func() error {
			attempts++
			regionsUsed = append(regionsUsed, vc.GetCurrentRegion())
			if attempts < 3 {
				return context.DeadlineExceeded
			}
			return nil
		}

		ctx := context.Background()
		err := executor.Execute(ctx, operation)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}

		expectedRegions := []string{"region-1", "region-2", "region-3"}
		for i, expected := range expectedRegions {
			if regionsUsed[i] != expected {
				t.Errorf("attempt[%d]: expected region %s, got %s", i, expected, regionsUsed[i])
			}
		}
	})

	t.Run("fails after max attempts", func(t *testing.T) {
		vc := &VertexConfig{
			Enabled: true,
			Region:  "us-east5,europe-west1",
		}
		vc.parseRegions()

		policy := &Policy{
			InitialInterval:    time.Millisecond * 10,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Second,
			MaximumAttempts:    3,
		}

		executor := NewVertexRetryExecutor(vc, policy)

		attempts := 0
		operation := func() error {
			attempts++
			return context.DeadlineExceeded
		}

		ctx := context.Background()
		err := executor.Execute(ctx, operation)

		if err == nil {
			t.Error("expected error after max attempts, got nil")
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("single region retries without rotation", func(t *testing.T) {
		vc := &VertexConfig{
			Enabled: true,
			Region:  "us-east5",
		}
		vc.parseRegions()

		policy := &Policy{
			InitialInterval:    time.Millisecond * 10,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Second,
			MaximumAttempts:    3,
		}

		executor := NewVertexRetryExecutor(vc, policy)

		attempts := 0
		regionsUsed := []string{}
		operation := func() error {
			attempts++
			regionsUsed = append(regionsUsed, vc.GetCurrentRegion())
			if attempts < 2 {
				return context.DeadlineExceeded
			}
			return nil
		}

		ctx := context.Background()
		err := executor.Execute(ctx, operation)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if attempts != 2 {
			t.Errorf("expected 2 attempts, got %d", attempts)
		}

		for i, region := range regionsUsed {
			if region != "us-east5" {
				t.Errorf("attempt[%d]: expected us-east5, got %s", i, region)
			}
		}
	})
}