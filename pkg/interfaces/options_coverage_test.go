package interfaces

import (
	"context"
	"reflect"
	"testing"
)

func TestQueryOptions(t *testing.T) {
	var o QueryOptions
	for _, opt := range []QueryOption{
		QueryWithLimit(10),
		QueryWithOffset(5),
		QueryWithOrderBy("created_at", "desc"),
	} {
		opt(&o)
	}
	want := QueryOptions{Limit: 10, Offset: 5, OrderBy: "created_at", OrderDirection: "desc"}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v, want %+v", o, want)
	}
}

func TestGraphStoreOptions(t *testing.T) {
	var o GraphStoreOptions
	for _, opt := range []GraphStoreOption{
		WithGraphBatchSize(50),
		WithGenerateEmbeddings(true),
		WithGraphTenant("acme"),
	} {
		opt(&o)
	}
	want := GraphStoreOptions{BatchSize: 50, GenerateEmbeddings: true, Tenant: "acme"}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v, want %+v", o, want)
	}
}

func TestGraphSearchOptions(t *testing.T) {
	var o GraphSearchOptions
	for _, opt := range []GraphSearchOption{
		WithMinGraphScore(0.7),
		WithEntityTypes("person", "org"),
		WithRelationshipTypes("works_at"),
		WithMaxDepth(3),
		WithIncludeRelationships(true),
		WithSearchTenant("acme"),
		WithSearchMode(SearchModeHybrid),
	} {
		opt(&o)
	}
	want := GraphSearchOptions{
		MinScore:             0.7,
		EntityTypes:          []string{"person", "org"},
		RelationshipTypes:    []string{"works_at"},
		MaxDepth:             3,
		IncludeRelationships: true,
		Tenant:               "acme",
		SearchMode:           SearchModeHybrid,
	}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v, want %+v", o, want)
	}
}

func TestExtractionOptions(t *testing.T) {
	var o ExtractionOptions
	for _, opt := range []ExtractionOption{
		WithSchemaGuided(true),
		WithExtractionEntityTypes("person"),
		WithExtractionRelationshipTypes("knows", "manages"),
		WithMinConfidence(0.5),
		WithMaxEntities(20),
		WithDedupThreshold(0.9),
	} {
		opt(&o)
	}
	want := ExtractionOptions{
		SchemaGuided:      true,
		EntityTypes:       []string{"person"},
		RelationshipTypes: []string{"knows", "manages"},
		MinConfidence:     0.5,
		MaxEntities:       20,
		DedupThreshold:    0.9,
	}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v, want %+v", o, want)
	}
}

func TestImageGenerationOptions(t *testing.T) {
	opts := DefaultImageGenerationOptions()
	wantDefault := ImageGenerationOptions{NumberOfImages: 1, AspectRatio: "1:1", OutputFormat: "png", SafetyFilterLevel: "medium"}
	if *opts != wantDefault {
		t.Fatalf("default = %+v, want %+v", *opts, wantDefault)
	}

	ApplyImageGenerationOptions(opts,
		WithNumberOfImages(3),
		WithAspectRatio("16:9"),
		WithOutputFormat("jpeg"),
		WithSafetyFilter("high"),
	)
	want := ImageGenerationOptions{NumberOfImages: 3, AspectRatio: "16:9", OutputFormat: "jpeg", SafetyFilterLevel: "high"}
	if *opts != want {
		t.Fatalf("applied = %+v, want %+v", *opts, want)
	}

	// No options leaves the value unchanged.
	ApplyImageGenerationOptions(opts)
	if *opts != want {
		t.Fatalf("no-op apply changed options to %+v", *opts)
	}
}

func TestDefaultImageEditOptions(t *testing.T) {
	got := DefaultImageEditOptions()
	if got.AspectRatio != "1:1" || got.ImageSize != "1K" {
		t.Fatalf("got %+v", *got)
	}
}

func TestGenerateOptions(t *testing.T) {
	mem := stubMemory{}
	format := ResponseFormat{Type: ResponseFormatJSON, Name: "Answer", Schema: JSONSchema{"type": "object"}}
	stream := StreamConfig{BufferSize: 7}

	o := GenerateOptions{}
	for _, opt := range []GenerateOption{
		WithMaxIterations(4),
		WithDisableFinalSummary(true),
		WithMemory(mem),
		WithStreamConfig(stream),
		WithSystemMessage("be brief"),
		WithFrequencyPenalty(0.3),
		WithPresencePenalty(0.4),
		WithStopSequences([]string{"END"}),
		WithResponseFormat(format),
		WithReasoning(true, 2048),
	} {
		opt(&o)
	}

	if o.MaxIterations != 4 || !o.DisableFinalSummary || o.SystemMessage != "be brief" {
		t.Fatalf("scalar options not applied: %+v", o)
	}
	if !reflect.DeepEqual(o.Memory, Memory(mem)) {
		t.Fatalf("memory not applied")
	}
	if o.StreamConfig == nil || o.StreamConfig.BufferSize != 7 {
		t.Fatalf("stream config = %+v", o.StreamConfig)
	}
	if o.ResponseFormat == nil || o.ResponseFormat.Name != "Answer" {
		t.Fatalf("response format = %+v", o.ResponseFormat)
	}
	want := LLMConfig{
		FrequencyPenalty: 0.3,
		PresencePenalty:  0.4,
		StopSequences:    []string{"END"},
		EnableReasoning:  true,
		ReasoningBudget:  2048,
	}
	if !reflect.DeepEqual(*o.LLMConfig, want) {
		t.Fatalf("llm config = %+v, want %+v", *o.LLMConfig, want)
	}
}

// Each LLMConfig option must allocate the config when it is nil.
func TestLLMConfigOptionsAllocate(t *testing.T) {
	tests := []struct {
		name  string
		opt   GenerateOption
		check func(*LLMConfig) bool
	}{
		{"top p", WithTopP(0.9), func(c *LLMConfig) bool { return c.TopP == 0.9 }},
		{"frequency penalty", WithFrequencyPenalty(1), func(c *LLMConfig) bool { return c.FrequencyPenalty == 1 }},
		{"presence penalty", WithPresencePenalty(1), func(c *LLMConfig) bool { return c.PresencePenalty == 1 }},
		{"stop sequences", WithStopSequences([]string{"x"}), func(c *LLMConfig) bool { return len(c.StopSequences) == 1 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o GenerateOptions
			tc.opt(&o)
			if o.LLMConfig == nil || !tc.check(o.LLMConfig) {
				t.Fatalf("got %+v", o.LLMConfig)
			}
		})
	}
}

func TestWithReasoningWithoutBudgetKeepsBudget(t *testing.T) {
	o := GenerateOptions{LLMConfig: &LLMConfig{ReasoningBudget: 1500}}
	WithReasoning(false)(&o)
	if o.LLMConfig.EnableReasoning || o.LLMConfig.ReasoningBudget != 1500 {
		t.Fatalf("got %+v", *o.LLMConfig)
	}
}

func TestGetMessagesOptions(t *testing.T) {
	var o GetMessagesOptions
	for _, opt := range []GetMessagesOption{WithLimit(3), WithRoles("user", "assistant"), WithQuery("hello")} {
		opt(&o)
	}
	want := GetMessagesOptions{Limit: 3, Roles: []string{"user", "assistant"}, Query: "hello"}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v, want %+v", o, want)
	}
}

func TestWithIncludeIntermediateMessages(t *testing.T) {
	cfg := DefaultStreamConfig()
	WithIncludeIntermediateMessages(true)(&cfg)
	if !cfg.IncludeIntermediateMessages {
		t.Fatal("expected intermediate messages to be enabled")
	}
	WithIncludeIntermediateMessages(false)(&cfg)
	if cfg.IncludeIntermediateMessages {
		t.Fatal("expected intermediate messages to be disabled")
	}
}

func TestStoreSearchDeleteOptions(t *testing.T) {
	var store StoreOptions
	for _, opt := range []StoreOption{WithBatchSize(100), WithGenerateVectors(true), WithClass("Docs"), WithTenant("t1")} {
		opt(&store)
	}
	wantStore := StoreOptions{BatchSize: 100, GenerateVectors: true, Class: "Docs", Tenant: "t1"}
	if !reflect.DeepEqual(store, wantStore) {
		t.Fatalf("store = %+v, want %+v", store, wantStore)
	}

	var search SearchOptions
	filters := map[string]interface{}{"lang": "en"}
	for _, opt := range []SearchOption{
		WithMinScore(0.2),
		WithFilters(filters),
		WithEmbedding(true),
		WithBM25(true),
		WithNearText(true),
		WithKeyword(true),
		WithTenantSearch("t2"),
		WithFields("title", "body"),
	} {
		opt(&search)
	}
	wantSearch := SearchOptions{
		MinScore:     0.2,
		Filters:      filters,
		UseEmbedding: true,
		UseBM25:      true,
		UseNearText:  true,
		UseKeyword:   true,
		Tenant:       "t2",
		Fields:       []string{"title", "body"},
	}
	if !reflect.DeepEqual(search, wantSearch) {
		t.Fatalf("search = %+v, want %+v", search, wantSearch)
	}

	var del DeleteOptions
	WithTenantDelete("t3")(&del)
	if del.Tenant != "t3" {
		t.Fatalf("delete = %+v", del)
	}
}

// convMemory implements ConversationMemory.
type convMemory struct{ stubMemory }

func (convMemory) GetAllConversations(context.Context) ([]string, error) { return nil, nil }
func (convMemory) GetConversationMessages(context.Context, string) ([]Message, error) {
	return nil, nil
}
func (convMemory) GetMemoryStatistics(context.Context) (int, int, error) { return 0, 0, nil }

// adminMemory implements AdminConversationMemory.
type adminMemory struct{ convMemory }

func (adminMemory) GetAllConversationsAcrossOrgs() (map[string][]string, error) { return nil, nil }
func (adminMemory) GetConversationMessagesAcrossOrgs(string) ([]Message, string, error) {
	return nil, "", nil
}
func (adminMemory) GetMemoryStatisticsAcrossOrgs() (int, int, error) { return 0, 0, nil }

// wrapMemory is a decorator that hides the capabilities of what it wraps.
type wrapMemory struct {
	stubMemory
	inner Memory
}

func (w wrapMemory) Unwrap() Memory { return w.inner }

func TestUnwrapMemory(t *testing.T) {
	base := stubMemory{}
	tests := []struct {
		name string
		in   Memory
		want Memory
	}{
		{"nil", nil, nil},
		{"plain", base, base},
		{"one layer", wrapMemory{inner: base}, base},
		{"two layers", wrapMemory{inner: wrapMemory{inner: base}}, base},
		{"wraps nil returns wrapper", wrapMemory{}, wrapMemory{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnwrapMemory(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestAsConversationMemory(t *testing.T) {
	tests := []struct {
		name string
		in   Memory
		ok   bool
	}{
		{"nil", nil, false},
		{"plain memory", stubMemory{}, false},
		{"direct", convMemory{}, true},
		{"wrapped", wrapMemory{inner: convMemory{}}, true},
		{"wrapped plain", wrapMemory{inner: stubMemory{}}, false},
		{"wraps nil", wrapMemory{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cm, ok := AsConversationMemory(tc.in)
			if ok != tc.ok || (cm != nil) != tc.ok {
				t.Fatalf("got (%v, %v), want ok=%v", cm, ok, tc.ok)
			}
		})
	}
}

func TestAsAdminConversationMemory(t *testing.T) {
	tests := []struct {
		name string
		in   Memory
		ok   bool
	}{
		{"nil", nil, false},
		{"conversation only", convMemory{}, false},
		{"direct", adminMemory{}, true},
		{"wrapped", wrapMemory{inner: wrapMemory{inner: adminMemory{}}}, true},
		{"wraps nil", wrapMemory{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			am, ok := AsAdminConversationMemory(tc.in)
			if ok != tc.ok || (am != nil) != tc.ok {
				t.Fatalf("got (%v, %v), want ok=%v", am, ok, tc.ok)
			}
		})
	}
}
