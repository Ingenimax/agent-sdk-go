package graphrag

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"entity", ErrEntityNotFound, true},
		{"relationship", ErrRelationshipNotFound, true},
		{"path", ErrPathNotFound, true},
		{"source", ErrSourceEntityNotFound, true},
		{"target", ErrTargetEntityNotFound, true},
		{"wrapped", fmt.Errorf("lookup: %w", ErrEntityNotFound), true},
		{"validation error", ErrInvalidEntityID, false},
		{"other error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotFoundError(tt.err))
		})
	}
}

func TestIsValidationError(t *testing.T) {
	validation := []error{
		ErrInvalidEntityID, ErrInvalidRelationshipID, ErrMissingEntityName,
		ErrMissingEntityType, ErrMissingSourceID, ErrMissingTargetID,
		ErrMissingRelationshipType, ErrInvalidStrength, ErrEmptyQuery,
		ErrInvalidDepth, ErrSchemaValidation,
	}
	for _, err := range validation {
		assert.True(t, IsValidationError(err), err.Error())
		assert.True(t, IsValidationError(fmt.Errorf("wrap: %w", err)), err.Error())
	}

	for _, err := range []error{ErrEntityNotFound, ErrNoLLM, ErrConnectionFailed, nil} {
		assert.False(t, IsValidationError(err))
	}
}

func TestStoreOptions(t *testing.T) {
	var opts GraphStoreOptions
	for _, o := range []GraphStoreOption{WithBatchSize(50), WithGenerateEmbeddings(true), WithTenant("acme")} {
		o(&opts)
	}
	assert.Equal(t, GraphStoreOptions{BatchSize: 50, GenerateEmbeddings: true, Tenant: "acme"}, opts)
}

func TestSearchOptions(t *testing.T) {
	var opts GraphSearchOptions
	for _, o := range []GraphSearchOption{
		WithMinScore(0.7),
		WithEntityTypes("Person", "Org"),
		WithRelationshipTypes("WORKS_AT"),
		WithMaxDepth(3),
		WithIncludeRelationships(true),
		WithSearchTenant("acme"),
		WithMode(SearchModeHybrid),
	} {
		o(&opts)
	}
	assert.Equal(t, GraphSearchOptions{
		MinScore:             0.7,
		EntityTypes:          []string{"Person", "Org"},
		RelationshipTypes:    []string{"WORKS_AT"},
		MaxDepth:             3,
		IncludeRelationships: true,
		Tenant:               "acme",
		SearchMode:           SearchModeHybrid,
	}, opts)
}

func TestExtractionOptions(t *testing.T) {
	var opts ExtractionOptions
	for _, o := range []ExtractionOption{
		WithSchemaGuided(true),
		WithExtractionEntityTypes("Person"),
		WithExtractionRelationshipTypes("KNOWS"),
		WithMinConfidence(0.5),
		WithMaxEntities(10),
		WithDedupThreshold(0.9),
	} {
		o(&opts)
	}
	assert.Equal(t, ExtractionOptions{
		SchemaGuided:      true,
		EntityTypes:       []string{"Person"},
		RelationshipTypes: []string{"KNOWS"},
		MinConfidence:     0.5,
		MaxEntities:       10,
		DedupThreshold:    0.9,
	}, opts)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, &Config{
		Provider:    "weaviate",
		Host:        "localhost:8080",
		Scheme:      "http",
		ClassPrefix: "Graph",
	}, cfg)
}
