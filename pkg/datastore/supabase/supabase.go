package supabase

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/supabase-community/postgrest-go"
	"github.com/supabase-community/supabase-go"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Client implements the DataStore interface for Supabase
type Client struct {
	supabase *supabase.Client
	db       *sql.DB
}

// Option represents an option for configuring the client
type Option func(*Client)

// WithDB sets the SQL database connection
func WithDB(db *sql.DB) Option {
	return func(c *Client) {
		c.db = db
	}
}

// New creates a new Supabase client
func New(url string, apiKey string, options ...Option) (*Client, error) {
	supabaseClient, err := supabase.NewClient(url, apiKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Supabase client: %w", err)
	}

	client := &Client{
		supabase: supabaseClient,
	}

	for _, option := range options {
		option(client)
	}

	return client, nil
}

// Collection returns a reference to a specific collection/table
func (c *Client) Collection(name string) interfaces.CollectionRef {
	return &Collection{
		client: c,
		name:   name,
	}
}

// Transaction executes multiple operations in a transaction
func (c *Client) Transaction(ctx context.Context, fn func(tx interfaces.Transaction) error) error {
	if c.db == nil {
		return errors.New("database connection is required for transactions")
	}

	// Start transaction
	sqlTx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Create transaction object
	tx := &Transaction{
		client: c,
		tx:     sqlTx,
	}

	// Execute transaction function
	if err := fn(tx); err != nil {
		// Rollback on error
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("transaction failed with error: %v, rollback failed with error: %w", err, rbErr)
		}
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Close closes the database connection
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// Collection represents a reference to a collection/table
type Collection struct {
	client *Client
	name   string
}

// Insert inserts a document into the collection
func (c *Collection) Insert(ctx context.Context, data map[string]interface{}) (string, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Add organization ID and created_at to data
	data["org_id"] = orgID
	data["created_at"] = time.Now()

	// Generate ID if not provided
	id, ok := data["id"].(string)
	if !ok || id == "" {
		id = uuid.New().String()
		data["id"] = id
	}

	// Insert data
	resp, _, err := c.client.supabase.From(c.name).Insert(data, false, "", "", "").Execute()
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	// Parse response to check for success
	var results []map[string]interface{}
	if err := json.Unmarshal(resp, &results); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no document was inserted")
	}

	return id, nil
}

// Get retrieves a document by ID
func (c *Collection) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Query document
	resp, _, err := c.client.supabase.From(c.name).
		Select("*", "", false).
		Eq("id", id).
		Eq("org_id", orgID).
		Execute()

	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	// Parse response
	var results []map[string]interface{}
	if err := json.Unmarshal(resp, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("document not found")
	}

	return results[0], nil
}

// Update updates a document by ID
func (c *Collection) Update(ctx context.Context, id string, data map[string]interface{}) error {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Add updated_at to data
	data["updated_at"] = time.Now()

	// Update document
	resp, _, err := c.client.supabase.From(c.name).
		Update(data, "", "").
		Eq("id", id).
		Eq("org_id", orgID).
		Execute()

	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	// Parse response to check for success
	var results []map[string]interface{}
	if err := json.Unmarshal(resp, &results); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no document was updated")
	}

	return nil
}

// Delete deletes a document by ID
func (c *Collection) Delete(ctx context.Context, id string) error {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Delete document
	resp, _, err := c.client.supabase.From(c.name).
		Delete("", "").
		Eq("id", id).
		Eq("org_id", orgID).
		Execute()

	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	// Parse response to check for success
	var results []map[string]interface{}
	if err := json.Unmarshal(resp, &results); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no document was deleted")
	}

	return nil
}

// Query queries documents in the collection
func (c *Collection) Query(ctx context.Context, filter map[string]interface{}, options ...interfaces.QueryOption) ([]map[string]interface{}, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Apply options
	opts := &interfaces.QueryOptions{}
	for _, option := range options {
		option(opts)
	}

	// Start query
	query := c.client.supabase.From(c.name).Select("*", "", false)

	// Add organization ID filter
	query = query.Eq("org_id", orgID)

	// Add filters
	for k, v := range filter {
		query = query.Eq(k, v.(string))
	}

	// Add limit and offset
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit, "")
	}
	// Offset is not supported by this API

	// Add order by
	if opts.OrderBy != "" {
		if strings.ToLower(opts.OrderDirection) == "desc" {
			query = query.Order(opts.OrderBy, &postgrest.OrderOpts{Ascending: false})
		} else {
			query = query.Order(opts.OrderBy, &postgrest.OrderOpts{Ascending: true})
		}
	}

	// Execute query
	resp, _, err := query.Execute()
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}

	// Parse response
	var results []map[string]interface{}
	if err := json.Unmarshal(resp, &results); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return results, nil
}

// Transaction represents a database transaction
type Transaction struct {
	client *Client
	tx     *sql.Tx
}

// Collection returns a reference to a specific collection/table within the transaction
func (t *Transaction) Collection(name string) interfaces.CollectionRef {
	return &TransactionCollection{
		tx:   t.tx,
		name: name,
	}
}

// Commit commits the transaction
func (t *Transaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *Transaction) Rollback() error {
	return t.tx.Rollback()
}

// TransactionCollection represents a collection within a transaction
type TransactionCollection struct {
	tx   *sql.Tx
	name string
}

// Insert inserts a document into the collection within a transaction
func (c *TransactionCollection) Insert(ctx context.Context, data map[string]interface{}) (string, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Add organization ID and created_at to data
	data["org_id"] = orgID
	data["created_at"] = time.Now()

	// Generate ID if not provided
	id, ok := data["id"].(string)
	if !ok || id == "" {
		id = uuid.New().String()
		data["id"] = id
	}

	// Build query
	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	i := 1

	for k, v := range data {
		columns = append(columns, k)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, v)
		i++
	}

	// Validate table name to prevent SQL injection
	if !isValidTableName(c.name) {
		return "", fmt.Errorf("invalid table name: %s", c.name)
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING id",
		c.name,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Execute query
	var returnedID string
	err = c.tx.QueryRowContext(ctx, query, values...).Scan(&returnedID)
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	return returnedID, nil
}

// Get retrieves a document by ID within a transaction
func (c *TransactionCollection) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Validate table name to prevent SQL injection
	if !isValidTableName(c.name) {
		return nil, fmt.Errorf("invalid table name: %s", c.name)
	}

	// Build query
	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE id = $1 AND org_id = $2",
		c.name,
	)

	// Execute query
	var result map[string]interface{}
	err = c.tx.QueryRowContext(ctx, query, id, orgID).Scan(&result)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("document not found")
		}
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	return result, nil
}

// Update updates a document by ID within a transaction
func (c *TransactionCollection) Update(ctx context.Context, id string, data map[string]interface{}) error {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Add updated_at to data
	data["updated_at"] = time.Now()

	// Validate table name to prevent SQL injection
	if !isValidTableName(c.name) {
		return fmt.Errorf("invalid table name: %s", c.name)
	}

	// Build query
	setStatements := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data)+2)
	i := 1

	for k, v := range data {
		setStatements = append(setStatements, fmt.Sprintf("%s = $%d", k, i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d AND org_id = $%d",
		c.name,
		strings.Join(setStatements, ", "),
		i,
		i+1,
	)

	values = append(values, id, orgID)

	// Execute query
	result, err := c.tx.ExecContext(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("document not found or not owned by organization")
	}

	return nil
}

// Delete deletes a document by ID within a transaction
func (c *TransactionCollection) Delete(ctx context.Context, id string) error {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Validate table name to prevent SQL injection
	if !isValidTableName(c.name) {
		return fmt.Errorf("invalid table name: %s", c.name)
	}

	// Build query
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE id = $1 AND org_id = $2",
		c.name,
	)

	// Execute query
	result, err := c.tx.ExecContext(ctx, query, id, orgID)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("document not found or not owned by organization")
	}

	return nil
}

// Query queries documents in the collection within a transaction
func (c *TransactionCollection) Query(ctx context.Context, filter map[string]interface{}, options ...interfaces.QueryOption) ([]map[string]interface{}, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Apply options
	opts := &interfaces.QueryOptions{}
	for _, option := range options {
		option(opts)
	}

	// Validate table name to prevent SQL injection
	if !isValidTableName(c.name) {
		return nil, fmt.Errorf("invalid table name: %s", c.name)
	}

	// Build query
	whereStatements := []string{"org_id = $1"}
	values := []interface{}{orgID}
	i := 2

	for k, v := range filter {
		whereStatements = append(whereStatements, fmt.Sprintf("%s = $%d", k, i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s",
		c.name,
		strings.Join(whereStatements, " AND "),
	)

	// Add order by
	if opts.OrderBy != "" {
		direction := "ASC"
		if strings.ToLower(opts.OrderDirection) == "desc" {
			direction = "DESC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", opts.OrderBy, direction)
	}

	// Add limit and offset
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	// Execute query
	rows, err := c.tx.QueryContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer rows.Close()

	// Parse results
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		result := make(map[string]interface{})
		for i, col := range columns {
			result[col] = values[i]
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// isValidTableName checks if a table name contains only allowed characters (letters, numbers, underscores)
// This prevents SQL injection through table names
func isValidTableName(name string) bool {
	// Only allow alphanumeric characters and underscores in table names
	validName := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	return validName.MatchString(name)
}
