package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Client implements the DataStore interface for PostgreSQL
type Client struct {
	db *sql.DB
}

// Option represents an option for configuring the client
type Option func(*Client)

// New creates a new PostgreSQL client
func New(connectionString string, options ...Option) (*Client, error) {
	// Connect to PostgreSQL database
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	client := &Client{
		db: db,
	}

	for _, option := range options {
		option(client)
	}

	return client, nil
}

// NewWithDB creates a new PostgreSQL client with an existing database connection
func NewWithDB(db *sql.DB) (*Client, error) {
	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	return &Client{
		db: db,
	}, nil
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
	return c.db.Close()
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

	// Build query
	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	i := 1

	for k, v := range data {
		columns = append(columns, pq.QuoteIdentifier(k))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING id", // #nosec G201
		pq.QuoteIdentifier(c.name),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Execute query
	var returnedID string
	err = c.client.db.QueryRowContext(ctx, query, values...).Scan(&returnedID)
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	return returnedID, nil
}

// Get retrieves a document by ID
func (c *Collection) Get(ctx context.Context, id string) (map[string]interface{}, error) {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Build query
	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE id = $1 AND org_id = $2",
		pq.QuoteIdentifier(c.name),
	)

	// Execute query
	rows, err := c.client.db.QueryContext(ctx, query, id, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query document: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cerr)
			}
		}
	}()

	// Get columns
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Scan result
	if !rows.Next() {
		return nil, fmt.Errorf("document not found")
	}

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

	return result, nil
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

	// Build query
	setStatements := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data)+2)
	i := 1

	for k, v := range data {
		setStatements = append(setStatements, fmt.Sprintf("%s = $%d", pq.QuoteIdentifier(k), i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d AND org_id = $%d", // #nosec G201
		pq.QuoteIdentifier(c.name),
		strings.Join(setStatements, ", "),
		i,
		i+1,
	)

	values = append(values, id, orgID)

	// Execute query
	result, err := c.client.db.ExecContext(ctx, query, values...)
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

// Delete deletes a document by ID
func (c *Collection) Delete(ctx context.Context, id string) error {
	// Get organization ID from context
	orgID, err := multitenancy.GetOrgID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get organization ID: %w", err)
	}

	// Build query
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE id = $1 AND org_id = $2",
		pq.QuoteIdentifier(c.name),
	)

	// Execute query
	result, err := c.client.db.ExecContext(ctx, query, id, orgID)
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

	// Build query
	whereStatements := []string{"org_id = $1"}
	values := []interface{}{orgID}
	i := 2

	for k, v := range filter {
		whereStatements = append(whereStatements, fmt.Sprintf("%s = $%d", pq.QuoteIdentifier(k), i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s", // #nosec G201 - table name is sanitized with pq.QuoteIdentifier and WHERE conditions use parameterized queries
		pq.QuoteIdentifier(c.name),
		strings.Join(whereStatements, " AND "),
	)

	// Add order by
	if opts.OrderBy != "" {
		direction := "ASC"
		if strings.ToLower(opts.OrderDirection) == "desc" {
			direction = "DESC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", pq.QuoteIdentifier(opts.OrderBy), direction)
	}

	// Add limit and offset
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	// Execute query
	rows, err := c.client.db.QueryContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("failed to query documents: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			// Merge with existing error or set if none
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cerr)
			}
		}
	}()

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
		columns = append(columns, pq.QuoteIdentifier(k))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING id", // #nosec G201
		pq.QuoteIdentifier(c.name),
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

	// Build query
	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE id = $1 AND org_id = $2",
		pq.QuoteIdentifier(c.name),
	)

	// Execute query
	rows, err := c.tx.QueryContext(ctx, query, id, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to query document: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cerr)
			}
		}
	}()

	// Get columns
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Scan result
	if !rows.Next() {
		return nil, fmt.Errorf("document not found")
	}

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

	// Build query
	setStatements := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data)+2)
	i := 1

	for k, v := range data {
		setStatements = append(setStatements, fmt.Sprintf("%s = $%d", pq.QuoteIdentifier(k), i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d AND org_id = $%d", // #nosec G201
		pq.QuoteIdentifier(c.name),
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

	// Build query
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE id = $1 AND org_id = $2",
		pq.QuoteIdentifier(c.name),
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

	// Build query
	whereStatements := []string{"org_id = $1"}
	values := []interface{}{orgID}
	i := 2

	for k, v := range filter {
		whereStatements = append(whereStatements, fmt.Sprintf("%s = $%d", pq.QuoteIdentifier(k), i))
		values = append(values, v)
		i++
	}

	query := fmt.Sprintf(
		"SELECT * FROM %s WHERE %s", // #nosec G201 - table name is sanitized with pq.QuoteIdentifier and WHERE conditions use parameterized queries
		pq.QuoteIdentifier(c.name),
		strings.Join(whereStatements, " AND "),
	)

	// Add order by
	if opts.OrderBy != "" {
		direction := "ASC"
		if strings.ToLower(opts.OrderDirection) == "desc" {
			direction = "DESC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", pq.QuoteIdentifier(opts.OrderBy), direction)
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
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			// Merge with existing error or set if none
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cerr)
			}
		}
	}()

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
