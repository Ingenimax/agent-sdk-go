package weaviate

import (
	"context"
	"fmt"
	"strconv"

	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/auth"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Ingenimax/agent-sdk-go/pkg/embedding"
	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/logging"
	"github.com/go-openapi/strfmt"
)

// Store implements the VectorStore interface for Weaviate
type Store struct {
	client         *weaviate.Client
	classPrefix    string
	embedder       embedding.Client
	distanceMetric string
	logger         logging.Logger
}

// Option represents an option for configuring the Weaviate store
type Option func(*Store)

// WithClassPrefix sets the class prefix for the Weaviate store
func WithClassPrefix(prefix string) Option {
	return func(s *Store) {
		s.classPrefix = prefix
	}
}

// WithEmbedder sets the embedder for the Weaviate store
func WithEmbedder(embedder embedding.Client) Option {
	return func(s *Store) {
		s.embedder = embedder
	}
}

// WithDistanceMetric sets the distance metric for the Weaviate store
func WithDistanceMetric(metric string) Option {
	return func(s *Store) {
		s.distanceMetric = metric
	}
}

// WithLogger sets the logger for the Weaviate store
func WithLogger(logger logging.Logger) Option {
	return func(s *Store) {
		s.logger = logger
	}
}

// New creates a new Weaviate store
func New(config *interfaces.VectorStoreConfig, options ...Option) *Store {
	// Create store with default options
	store := &Store{
		classPrefix:    "Document",
		distanceMetric: "cosine",
		logger:         logging.New(),
	}

	// Apply options
	for _, option := range options {
		option(store)
	}

	// Create Weaviate client
	cfg := weaviate.Config{
		Host:   config.Host,
		Scheme: config.Scheme,
	}

	// Add API key if provided - use AuthConfig for proper Weaviate Cloud support
	if config.APIKey != "" {
		cfg.AuthConfig = auth.ApiKey{Value: config.APIKey}
	}

	client, err := weaviate.NewClient(cfg)
	if err != nil {
		store.logger.Error(context.Background(), "Failed to create Weaviate client", map[string]interface{}{"error": err.Error()})
		return nil
	}

	store.client = client

	return store
}

// getClassName returns the class name
// Uses metadata-based multi-tenancy (single class, orgId as field) instead of class proliferation
func (s *Store) getClassName(ctx context.Context, class string) (string, error) {
	// If class is provided, use it; otherwise use default
	if class == "" {
		class = s.classPrefix
	}

	// Always return the base class name
	// Multi-tenancy is handled via orgId field filtering, not separate classes
	s.logger.Debug(ctx, "Using single class with metadata-based multi-tenancy", map[string]interface{}{
		"class": class,
	})
	return class, nil
}

// Store stores documents in Weaviate with optional tenant support
func (s *Store) Store(ctx context.Context, documents []interfaces.Document, options ...interfaces.StoreOption) error {
	// Apply options
	opts := &interfaces.StoreOptions{
		BatchSize: 100,
	}
	for _, option := range options {
		option(opts)
	}

	// Get class name
	className, err := s.getClassName(ctx, opts.Class)
	if err != nil {
		return err
	}

	// Store documents in batches
	batch := s.client.Batch().ObjectsBatcher()
	batchSize := opts.BatchSize
	batchCount := 0

	for _, doc := range documents {
		// Generate embedding for the document content
		vector, err := s.embedder.Embed(ctx, doc.Content)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}

		properties := map[string]interface{}{
			"content": doc.Content,
		}
		for k, v := range doc.Metadata {
			properties[k] = v
		}

		obj := &models.Object{
			Class:      className,
			ID:         strfmt.UUID(doc.ID),
			Properties: properties,
			Vector:     vector, // Use the generated vector
		}

		// Add tenant support if specified
		if opts.Tenant != "" {
			obj.Tenant = opts.Tenant
		}

		batch.WithObjects(obj)
		batchCount++

		// Execute batch when it reaches the batch size
		if batchCount >= batchSize {
			if _, err := batch.Do(ctx); err != nil {
				return fmt.Errorf("failed to store batch: %w", err)
			}
			// Reset batch and count
			batch = s.client.Batch().ObjectsBatcher()
			batchCount = 0
		}
	}

	// Final batch
	if batchCount > 0 {
		if _, err := batch.Do(ctx); err != nil {
			return fmt.Errorf("failed to store final batch: %w", err)
		}
	}

	return nil
}

// Search searches for similar documents
func (s *Store) Search(ctx context.Context, query string, limit int, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
	// Apply options
	opts := &interfaces.SearchOptions{
		MinScore: 0.0,
	}
	for _, option := range options {
		option(opts)
	}

	// Get class name
	className, err := s.getClassName(ctx, opts.Class)
	if err != nil {
		return nil, err
	}

	// Generate embedding for the query
	vector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for query: %w", err)
	}

	// Build query
	whereFilter := s.buildWhereFilter(opts.Filters)

	// Debug log for filter
	if len(opts.Filters) > 0 {
		s.logger.Info(ctx, "Applying filters", map[string]interface{}{"filters": opts.Filters})
		if whereFilter != nil {
			s.logger.Info(ctx, "Built where filter", map[string]interface{}{"filter": whereFilter})
		} else {
			s.logger.Info(ctx, "Warning: Failed to build where filter from filters", nil)
		}
	}

	// Log the GraphQL query details
	s.logger.Info(ctx, "Executing GraphQL query", map[string]interface{}{
		"className": className,
		"limit":     limit,
		"query":     query,
	})

	// Build dynamic field list
	fieldList, err := s.buildFieldList(ctx, className, opts.Fields)
	if err != nil {
		return nil, fmt.Errorf("failed to build field list: %w", err)
	}

	s.logger.Debug(ctx, "Using field list for search", map[string]interface{}{
		"fieldList": fieldList,
		"className": className,
	})

	// Build query with dynamic fields
	queryBuilder := s.client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: fieldList,
		}).
		WithNearVector(s.client.GraphQL().NearVectorArgBuilder().
			WithVector(vector)).
		WithLimit(limit)

	// Add where filter if specified
	if whereFilter != nil {
		queryBuilder = queryBuilder.WithWhere(whereFilter)
	}

	// Add tenant support if specified
	if opts.Tenant != "" {
		queryBuilder = queryBuilder.WithTenant(opts.Tenant)
	}

	result, err := queryBuilder.Do(ctx)

	if err != nil {
		s.logger.Error(ctx, "GraphQL query failed", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Log the raw response for debugging
	s.logger.Info(ctx, "GraphQL response received", map[string]interface{}{
		"rawData": result.Data,
		"errors":  result.Errors,
	})

	// Parse results
	searchResults, err := s.parseSearchResults(result, className)
	if err != nil {
		return nil, err
	}

	// Apply similarity threshold
	filteredResults := []interfaces.SearchResult{}
	for _, res := range searchResults {
		if res.Score >= opts.MinScore {
			filteredResults = append(filteredResults, res)
		}
	}

	return filteredResults, nil
}

// SearchByVector searches for similar documents using a vector
func (s *Store) SearchByVector(ctx context.Context, vector []float32, limit int, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
	// Apply options
	opts := &interfaces.SearchOptions{
		MinScore: 0.0,
	}
	for _, option := range options {
		option(opts)
	}

	// Get class name
	className, err := s.getClassName(ctx, opts.Class)
	if err != nil {
		return nil, err
	}

	// Build query
	whereFilter := s.buildWhereFilter(opts.Filters)

	// Build dynamic field list
	fieldList, err := s.buildFieldList(ctx, className, opts.Fields)
	if err != nil {
		return nil, fmt.Errorf("failed to build field list: %w", err)
	}

	s.logger.Debug(ctx, "Using field list for vector search", map[string]interface{}{
		"fieldList": fieldList,
		"className": className,
	})

	// Use vector search
	queryBuilder := s.client.GraphQL().Get().
		WithClassName(className).
		WithFields(graphql.Field{
			Name: fieldList,
		}).
		WithNearVector(s.client.GraphQL().NearVectorArgBuilder().
			WithVector(vector)).
		WithWhere(whereFilter).
		WithLimit(limit)

	// Add tenant support if specified
	if opts.Tenant != "" {
		queryBuilder = queryBuilder.WithTenant(opts.Tenant)
	}

	result, err := queryBuilder.Do(ctx)
	if err != nil {
		s.logger.Error(ctx, "GraphQL query failed", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to execute vector search: %w", err)
	}

	// Parse results
	return s.parseSearchResults(result, className)
}

// Delete removes documents from Weaviate
func (s *Store) Delete(ctx context.Context, ids []string, options ...interfaces.DeleteOption) error {
	// Apply options
	opts := &interfaces.DeleteOptions{}
	for _, option := range options {
		option(opts)
	}

	// Get class name
	className, err := s.getClassName(ctx, opts.Class)
	if err != nil {
		return err
	}

	// Delete objects
	for _, id := range ids {
		deleter := s.client.Data().Deleter().
			WithClassName(className).
			WithID(id)

		// Add tenant support if specified
		if opts.Tenant != "" {
			deleter = deleter.WithTenant(opts.Tenant)
		}

		if err := deleter.Do(ctx); err != nil {
			return fmt.Errorf("failed to delete document %s: %w", id, err)
		}
	}

	return nil
}

// Get retrieves a single document by ID
func (s *Store) Get(ctx context.Context, id string, options ...interfaces.StoreOption) (*interfaces.Document, error) {
	// Apply options
	opts := &interfaces.StoreOptions{}
	for _, option := range options {
		option(opts)
	}

	// Get class name (use default since we're getting by ID)
	className, err := s.getClassName(ctx, opts.Class)
	if err != nil {
		return nil, err
	}

	getter := s.client.Data().ObjectsGetter().
		WithClassName(className).
		WithID(id)

	// Add tenant support if specified
	if opts.Tenant != "" {
		getter = getter.WithTenant(opts.Tenant)
	}

	result, err := getter.Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get document %s: %w", id, err)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("document %s not found", id)
	}

	doc := &interfaces.Document{
		ID:       id,
		Content:  result[0].Properties.(map[string]interface{})["content"].(string),
		Metadata: make(map[string]interface{}),
	}

	// Copy all properties except content to metadata
	for k, v := range result[0].Properties.(map[string]interface{}) {
		if k != "content" {
			doc.Metadata[k] = v
		}
	}

	return doc, nil
}

// GlobalStore stores documents in Weaviate without tenant context (for shared data)
func (s *Store) GlobalStore(ctx context.Context, documents []interfaces.Document, options ...interfaces.StoreOption) error {
	// Create a context without organization ID to ensure global storage
	globalCtx := context.Background()
	return s.Store(globalCtx, documents, options...)
}

// GlobalSearch searches for documents without tenant context (for shared data)
func (s *Store) GlobalSearch(ctx context.Context, query string, limit int, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
	// Create a context without organization ID to ensure global search
	globalCtx := context.Background()
	return s.Search(globalCtx, query, limit, options...)
}

// GlobalSearchByVector searches for documents by vector without tenant context (for shared data)
func (s *Store) GlobalSearchByVector(ctx context.Context, vector []float32, limit int, options ...interfaces.SearchOption) ([]interfaces.SearchResult, error) {
	// Create a context without organization ID to ensure global search
	globalCtx := context.Background()
	return s.SearchByVector(globalCtx, vector, limit, options...)
}

// GlobalDelete deletes documents without tenant context (for shared data)
func (s *Store) GlobalDelete(ctx context.Context, ids []string, options ...interfaces.DeleteOption) error {
	// Create a context without organization ID to ensure global deletion
	globalCtx := context.Background()
	return s.Delete(globalCtx, ids, options...)
}

// CreateTenant creates a new tenant for native multi-tenancy
func (s *Store) CreateTenant(ctx context.Context, tenantName string) error {
	// Use the default class for tenant creation
	className, err := s.getClassName(ctx, "")
	if err != nil {
		return err
	}

	// Create tenant using the Weaviate Go client
	tenant := models.Tenant{
		Name: tenantName,
	}

	err = s.client.Schema().TenantsCreator().
		WithClassName(className).
		WithTenants(tenant).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to create tenant %s: %w", tenantName, err)
	}

	s.logger.Info(ctx, "Tenant created successfully", map[string]interface{}{
		"tenantName": tenantName,
		"className":  className,
	})
	return nil
}

// DeleteTenant deletes a tenant for native multi-tenancy
func (s *Store) DeleteTenant(ctx context.Context, tenantName string) error {
	// Use the default class for tenant deletion
	className, err := s.getClassName(ctx, "")
	if err != nil {
		return err
	}

	err = s.client.Schema().TenantsDeleter().
		WithClassName(className).
		WithTenants(tenantName).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete tenant %s: %w", tenantName, err)
	}

	s.logger.Info(ctx, "Tenant deleted successfully", map[string]interface{}{
		"tenantName": tenantName,
		"className":  className,
	})
	return nil
}

// ListTenants lists all tenants for native multi-tenancy
func (s *Store) ListTenants(ctx context.Context) ([]string, error) {
	// Use the default class for tenant listing
	className, err := s.getClassName(ctx, "")
	if err != nil {
		return nil, err
	}

	tenants, err := s.client.Schema().TenantsGetter().
		WithClassName(className).
		Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	var tenantNames []string
	for _, tenant := range tenants {
		tenantNames = append(tenantNames, tenant.Name)
	}

	s.logger.Info(ctx, "Tenants listed successfully", map[string]interface{}{
		"className": className,
		"count":     len(tenantNames),
	})
	return tenantNames, nil
}

// Helper functions

// buildFieldList constructs the GraphQL field specification for queries
// If fields are specified in options, uses those; otherwise discovers all fields from schema
func (s *Store) buildFieldList(ctx context.Context, className string, fields []string) (string, error) {
	// If specific fields are requested, use them
	if len(fields) > 0 {
		fieldList := ""
		for i, field := range fields {
			if i > 0 {
				fieldList += " "
			}
			fieldList += field
		}
		// Always include _additional metadata
		fieldList += " _additional { certainty id }"
		return fieldList, nil
	}

	// Auto-discover all fields from schema
	schema, err := s.client.Schema().Getter().Do(ctx)
	if err != nil {
		s.logger.Error(ctx, "Failed to get schema for field discovery", map[string]interface{}{
			"error":     err.Error(),
			"className": className,
		})
		// Fallback to basic fields if schema discovery fails
		return "content _additional { certainty id }", nil
	}

	// Find the target class
	var targetClass *models.Class
	for _, class := range schema.Classes {
		if class.Class == className {
			targetClass = class
			break
		}
	}

	if targetClass == nil {
		s.logger.Warn(ctx, "Class not found in schema, using fallback fields", map[string]interface{}{
			"className": className,
		})
		// Fallback to basic fields if class not found
		return "content _additional { certainty id }", nil
	}

	// Build field list from all properties
	fieldList := ""
	for i, property := range targetClass.Properties {
		if i > 0 {
			fieldList += " "
		}
		fieldList += property.Name
	}

	// Always include _additional metadata
	fieldList += " _additional { certainty id }"

	s.logger.Debug(ctx, "Built dynamic field list", map[string]interface{}{
		"className":  className,
		"fieldList":  fieldList,
		"fieldCount": len(targetClass.Properties),
	})

	return fieldList, nil
}

func (s *Store) buildWhereFilter(filterMap map[string]interface{}) *filters.WhereBuilder {
	if len(filterMap) == 0 {
		return nil
	}

	// Check for operands
	operandsIface, hasOperands := filterMap["operands"]
	if hasOperands {
		operator, hasOperator := filterMap["operator"]
		if !hasOperator {
			s.logger.Info(context.Background(), "Warning: Filter with operands missing operator", map[string]interface{}{"filter": filterMap})
			return nil
		}

		// Convert operands to a slice of filters
		operandsSlice, ok := operandsIface.([]interface{})
		if !ok {
			s.logger.Info(context.Background(), "Warning: Operands is not a slice", map[string]interface{}{"operands": operandsIface})
			return nil
		}

		// Build operands
		var whereOperands []*filters.WhereBuilder
		for _, operand := range operandsSlice {
			if subFilter := s.buildWhereFilter(operand.(map[string]interface{})); subFilter != nil {
				whereOperands = append(whereOperands, subFilter)
			}
		}

		// Create filter with operands
		if len(whereOperands) > 0 {
			switch operator {
			case "And":
				return filters.Where().WithOperator(filters.And).WithOperands(whereOperands)
			case "Or":
				return filters.Where().WithOperator(filters.Or).WithOperands(whereOperands)
			default:
				s.logger.Info(context.Background(), "Warning: Unsupported operator in filter with operands", map[string]interface{}{"operator": operator})
				return nil
			}
		}
		return nil
	}

	// Direct filter
	if len(filterMap) > 0 {
		operator, hasOperator := filterMap["operator"]
		if !hasOperator {
			s.logger.Info(context.Background(), "Warning: Direct filter missing operator", map[string]interface{}{"filter": filterMap})
			return nil
		}

		// Create the filter
		condition := filters.Where()

		// Handle path
		if pathSlice, ok := filterMap["path"].([]string); ok {
			condition = condition.WithPath(pathSlice)
		} else if pathStr, ok := filterMap["path"].(string); ok {
			condition = condition.WithPath([]string{pathStr})
		} else if pathIface, ok := filterMap["path"].([]interface{}); ok {
			pathSlice := make([]string, len(pathIface))
			for i, p := range pathIface {
				pathSlice[i] = fmt.Sprint(p)
			}
			condition = condition.WithPath(pathSlice)
		}

		// Handle operator and value
		switch operator {
		case "Equal":
			if val, ok := filterMap["valueString"]; ok {
				return condition.WithOperator(filters.Equal).WithValueString(fmt.Sprint(val))
			}
		case "NotEqual":
			if val, ok := filterMap["valueString"]; ok {
				return condition.WithOperator(filters.NotEqual).WithValueString(fmt.Sprint(val))
			}
		case "GreaterThan":
			if val, ok := filterMap["valueNumber"]; ok {
				return condition.WithOperator(filters.GreaterThan).WithValueNumber(toFloat64(val))
			}
		case "GreaterThanEqual":
			if val, ok := filterMap["valueNumber"]; ok {
				return condition.WithOperator(filters.GreaterThanEqual).WithValueNumber(toFloat64(val))
			}
		case "LessThan":
			if val, ok := filterMap["valueNumber"]; ok {
				return condition.WithOperator(filters.LessThan).WithValueNumber(toFloat64(val))
			}
		case "LessThanEqual":
			if val, ok := filterMap["valueNumber"]; ok {
				return condition.WithOperator(filters.LessThanEqual).WithValueNumber(toFloat64(val))
			}
		case "Like":
			if val, ok := filterMap["valueString"]; ok {
				return condition.WithOperator(filters.Like).WithValueString(fmt.Sprint(val))
			}
		case "ContainsAny":
			if val, ok := filterMap["valueString"]; ok {
				if strSlice, ok := val.([]string); ok {
					return condition.WithOperator(filters.ContainsAny).WithValueString(strSlice...)
				} else if strIface, ok := val.([]interface{}); ok {
					strSlice := make([]string, len(strIface))
					for i, s := range strIface {
						strSlice[i] = fmt.Sprint(s)
					}
					return condition.WithOperator(filters.ContainsAny).WithValueString(strSlice...)
				} else {
					return condition.WithOperator(filters.ContainsAny).WithValueString(fmt.Sprint(val))
				}
			}
		}

		s.logger.Info(context.Background(), "Warning: Could not build direct filter", map[string]interface{}{"filter": filterMap})
		return nil
	}

	// Check for logical operators (and, or)
	if andConditions, ok := filterMap["and"].([]interface{}); ok {
		// Create conditions for each operand
		var operands []*filters.WhereBuilder

		// Process each condition in the AND array
		for _, condition := range andConditions {
			// Check if this is a direct Weaviate filter
			if condMap, ok := condition.(map[string]interface{}); ok {
				if _, hasPath := condMap["path"]; hasPath {
					// This is a direct Weaviate filter
					if subFilter := s.buildWhereFilter(condMap); subFilter != nil {
						operands = append(operands, subFilter)
					}
					continue
				}

				// Otherwise, process as our custom filter format
				for field, value := range condMap {
					if valueMap, ok := value.(map[string]interface{}); ok {
						// Get operator and value from the map
						operator := valueMap["operator"].(string)
						val := valueMap["value"]

						// Create a condition for this field
						condition := filters.Where().
							WithPath([]string{field})

						// Apply the appropriate operator
						switch operator {
						case "equals":
							condition = condition.WithOperator(filters.Equal).WithValueString(fmt.Sprint(val))
						case "notEquals":
							condition = condition.WithOperator(filters.NotEqual).WithValueString(fmt.Sprint(val))
						case "greaterThan":
							condition = condition.WithOperator(filters.GreaterThan).WithValueNumber(toFloat64(val))
						case "greaterThanEqual":
							condition = condition.WithOperator(filters.GreaterThanEqual).WithValueNumber(toFloat64(val))
						case "lessThan":
							condition = condition.WithOperator(filters.LessThan).WithValueNumber(toFloat64(val))
						case "lessThanEqual":
							condition = condition.WithOperator(filters.LessThanEqual).WithValueNumber(toFloat64(val))
						case "like", "contains":
							condition = condition.WithOperator(filters.Like).WithValueString(fmt.Sprint(val))
						case "in":
							// Handle 'in' operator if supported by your Weaviate version
							if values, ok := val.([]interface{}); ok {
								strValues := make([]string, len(values))
								for i, v := range values {
									strValues[i] = fmt.Sprint(v)
								}
								// Use the correct method for ContainsAny operator
								condition = condition.WithOperator(filters.ContainsAny).WithValueString(strValues...)
							}
						}

						// Add this condition to the operands
						operands = append(operands, condition)
					}
				}
			}
		}

		// Create the AND group with all operands
		if len(operands) > 0 {
			return filters.Where().WithOperator(filters.And).WithOperands(operands)
		}
		return nil
	} else if orConditions, ok := filterMap["or"].([]interface{}); ok {
		// Create conditions for each operand
		var operands []*filters.WhereBuilder

		// Process each condition in the OR array
		for _, condition := range orConditions {
			// Check if this is a direct Weaviate filter
			if condMap, ok := condition.(map[string]interface{}); ok {
				if _, hasPath := condMap["path"]; hasPath {
					// This is a direct Weaviate filter
					if subFilter := s.buildWhereFilter(condMap); subFilter != nil {
						operands = append(operands, subFilter)
					}
					continue
				}

				// Otherwise, process as our custom filter format
				for field, value := range condMap {
					if valueMap, ok := value.(map[string]interface{}); ok {
						// Get operator and value from the map
						operator := valueMap["operator"].(string)
						val := valueMap["value"]

						// Create a condition for this field
						condition := filters.Where().
							WithPath([]string{field})

						// Apply the appropriate operator
						switch operator {
						case "equals":
							condition = condition.WithOperator(filters.Equal).WithValueString(fmt.Sprint(val))
						case "notEquals":
							condition = condition.WithOperator(filters.NotEqual).WithValueString(fmt.Sprint(val))
						case "greaterThan":
							condition = condition.WithOperator(filters.GreaterThan).WithValueNumber(toFloat64(val))
						case "greaterThanEqual":
							condition = condition.WithOperator(filters.GreaterThanEqual).WithValueNumber(toFloat64(val))
						case "lessThan":
							condition = condition.WithOperator(filters.LessThan).WithValueNumber(toFloat64(val))
						case "lessThanEqual":
							condition = condition.WithOperator(filters.LessThanEqual).WithValueNumber(toFloat64(val))
						case "like", "contains":
							condition = condition.WithOperator(filters.Like).WithValueString(fmt.Sprint(val))
						}

						// Add this condition to the operands
						operands = append(operands, condition)
					}
				}
			}
		}

		// Create the OR group with all operands
		if len(operands) > 0 {
			return filters.Where().WithOperator(filters.Or).WithOperands(operands)
		}
		return nil
	} else {
		// Handle simple key-value filters
		for field, value := range filterMap {
			if valueMap, ok := value.(map[string]interface{}); ok {
				operator := valueMap["operator"].(string)
				val := valueMap["value"]

				where := filters.Where().WithPath([]string{field})

				switch operator {
				case "equals":
					return where.WithOperator(filters.Equal).WithValueString(fmt.Sprint(val))
				case "notEquals":
					return where.WithOperator(filters.NotEqual).WithValueString(fmt.Sprint(val))
				case "greaterThan":
					return where.WithOperator(filters.GreaterThan).WithValueNumber(toFloat64(val))
				case "greaterThanEqual":
					return where.WithOperator(filters.GreaterThanEqual).WithValueNumber(toFloat64(val))
				case "lessThan":
					return where.WithOperator(filters.LessThan).WithValueNumber(toFloat64(val))
				case "lessThanEqual":
					return where.WithOperator(filters.LessThanEqual).WithValueNumber(toFloat64(val))
				case "like", "contains":
					return where.WithOperator(filters.Like).WithValueString(fmt.Sprint(val))
				}
			} else {
				// Simple equality
				return filters.Where().
					WithPath([]string{field}).
					WithOperator(filters.Equal).
					WithValueString(fmt.Sprint(value))
			}
		}
	}
	return nil
}

// Helper function to convert interface{} to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func (s *Store) parseSearchResults(result *models.GraphQLResponse, className string) ([]interfaces.SearchResult, error) {
	var searchResults []interfaces.SearchResult

	// Add debug logging
	s.logger.Info(context.Background(), "Parsing search results", map[string]interface{}{
		"className":  className,
		"resultData": result.Data,
	})

	// Check if result.Data is nil
	if result.Data == nil {
		s.logger.Warn(context.Background(), "Empty response data from Weaviate", nil)
		return []interfaces.SearchResult{}, nil // Return empty results instead of error
	}

	// Get the results array
	getMap, ok := result.Data["Get"].(map[string]interface{})
	if !ok {
		// Log the actual structure for debugging
		s.logger.Error(context.Background(), "Invalid response format", map[string]interface{}{
			"data": result.Data,
		})
		// Return empty results instead of error for production use
		return []interfaces.SearchResult{}, nil
	}

	results, ok := getMap[className].([]interface{})
	if !ok {
		// Return empty results if no matches found
		s.logger.Info(context.Background(), "No results found for class", map[string]interface{}{
			"className": className,
			"getMap":    getMap,
		})
		return searchResults, nil
	}

	for _, r := range results {
		result := r.(map[string]interface{})
		additional, ok := result["_additional"].(map[string]interface{})
		if !ok {
			s.logger.Warn(context.Background(), "Missing _additional field in result", map[string]interface{}{
				"result": result,
			})
			continue
		}

		content, ok := result["content"].(string)
		if !ok {
			s.logger.Warn(context.Background(), "Missing content field in result", map[string]interface{}{
				"result": result,
			})
			continue
		}

		id, ok := additional["id"].(string)
		if !ok {
			s.logger.Warn(context.Background(), "Missing id field in result", map[string]interface{}{
				"additional": additional,
			})
			continue
		}

		certainty, ok := additional["certainty"].(float64)
		if !ok {
			s.logger.Warn(context.Background(), "Missing certainty field in result", map[string]interface{}{
				"additional": additional,
			})
			// Use a default certainty value
			certainty = 0.5
		}

		doc := interfaces.Document{
			ID:       id,
			Content:  content,
			Metadata: make(map[string]interface{}),
		}

		// Copy all properties except content and _additional to metadata
		for k, v := range result {
			if k != "content" && k != "_additional" {
				doc.Metadata[k] = v
			}
		}

		searchResults = append(searchResults, interfaces.SearchResult{
			Document: doc,
			Score:    float32(certainty),
		})
	}

	return searchResults, nil
}
