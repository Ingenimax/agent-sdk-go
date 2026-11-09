package graphrag

import (
	"context"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// fakeStore records the calls the tools make. Methods the tools do not use
// come from the embedded nil interface and panic if called.
type fakeStore struct {
	interfaces.GraphRAGStore

	storedEntities      []interfaces.Entity
	storedRelationships []interfaces.Relationship
	storeEntitiesErr    error
	storeRelsErr        error

	searchResults []interfaces.GraphSearchResult
	searchErr     error
	lastMethod    string
	lastQuery     string
	lastLimit     int
	lastDepth     int
	lastLevel     int
	lastEntityID  string
	searchOpts    interfaces.GraphSearchOptions

	traverseResult *interfaces.GraphContext
	traverseErr    error

	extractResult *interfaces.ExtractionResult
	extractErr    error
	extractText   string
	extractLLM    interfaces.LLM
	extractOpts   interfaces.ExtractionOptions
}

func applySearchOpts(opts []interfaces.GraphSearchOption) interfaces.GraphSearchOptions {
	var o interfaces.GraphSearchOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func (f *fakeStore) StoreEntities(_ context.Context, entities []interfaces.Entity, _ ...interfaces.GraphStoreOption) error {
	if f.storeEntitiesErr != nil {
		return f.storeEntitiesErr
	}
	f.storedEntities = append(f.storedEntities, entities...)
	return nil
}

func (f *fakeStore) StoreRelationships(_ context.Context, rels []interfaces.Relationship, _ ...interfaces.GraphStoreOption) error {
	if f.storeRelsErr != nil {
		return f.storeRelsErr
	}
	f.storedRelationships = append(f.storedRelationships, rels...)
	return nil
}

func (f *fakeStore) Search(_ context.Context, query string, limit int, opts ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	f.lastMethod, f.lastQuery, f.lastLimit = "Search", query, limit
	f.searchOpts = applySearchOpts(opts)
	return f.searchResults, f.searchErr
}

func (f *fakeStore) LocalSearch(_ context.Context, query, entityID string, depth int, opts ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	f.lastMethod, f.lastQuery, f.lastEntityID, f.lastDepth = "LocalSearch", query, entityID, depth
	f.searchOpts = applySearchOpts(opts)
	return f.searchResults, f.searchErr
}

func (f *fakeStore) GlobalSearch(_ context.Context, query string, level int, opts ...interfaces.GraphSearchOption) ([]interfaces.GraphSearchResult, error) {
	f.lastMethod, f.lastQuery, f.lastLevel = "GlobalSearch", query, level
	f.searchOpts = applySearchOpts(opts)
	return f.searchResults, f.searchErr
}

func (f *fakeStore) TraverseFrom(_ context.Context, entityID string, depth int, opts ...interfaces.GraphSearchOption) (*interfaces.GraphContext, error) {
	f.lastMethod, f.lastEntityID, f.lastDepth = "TraverseFrom", entityID, depth
	f.searchOpts = applySearchOpts(opts)
	return f.traverseResult, f.traverseErr
}

func (f *fakeStore) ExtractFromText(_ context.Context, text string, llm interfaces.LLM, opts ...interfaces.ExtractionOption) (*interfaces.ExtractionResult, error) {
	f.extractText, f.extractLLM = text, llm
	f.extractOpts = interfaces.ExtractionOptions{}
	for _, opt := range opts {
		opt(&f.extractOpts)
	}
	return f.extractResult, f.extractErr
}

// fakeLLM is a placeholder LLM; the extract tool only passes it through.
type fakeLLM struct {
	interfaces.LLM
}
