# DataStore Examples

This directory contains examples for using different DataStore implementations with the Agent SDK.

## Available DataStore Clients

### 1. PostgreSQL (`/postgres`)

Pure PostgreSQL implementation using direct SQL connections.

**Best for:**
- Self-hosted PostgreSQL databases
- Full control over database configuration
- Existing PostgreSQL infrastructure
- Custom database optimizations
- On-premise deployments

**Features:**
- Direct SQL queries via `database/sql` and `lib/pq`
- Full transaction support
- All query options (limit, offset, ordering)
- Multi-tenancy with organization isolation
- Automatic timestamp management

**Setup:**
```bash
export POSTGRES_URL="postgres://user:password@localhost:5432/dbname?sslmode=disable"
cd postgres
go run main.go
```

[View PostgreSQL Example →](postgres/)

### 2. Supabase (`/supabase`)

Supabase implementation using REST API and SQL connections.

**Best for:**
- Cloud-hosted databases
- Rapid development
- Built-in authentication and authorization
- Real-time features
- Managed infrastructure

**Features:**
- REST API for CRUD operations
- Direct SQL for transactions
- Row Level Security (RLS) support
- Built-in authentication integration
- Real-time subscriptions (can be added)
- Multi-tenancy with organization isolation

**Setup:**
```bash
export SUPABASE_URL="https://your-project.supabase.co"
export SUPABASE_API_KEY="your-api-key"
export SUPABASE_DB_URL="postgresql://postgres:[PASSWORD]@db.your-project.supabase.co:5432/postgres"
cd supabase
go run main.go
```

[View Supabase Example →](supabase/)

## Choosing a DataStore

| Feature | PostgreSQL | Supabase |
|---------|-----------|----------|
| Setup Complexity | Medium | Easy |
| Hosting | Self-hosted | Cloud-managed |
| Authentication | Custom | Built-in |
| Real-time | Custom | Built-in |
| Transaction Support | ✅ Full | ✅ Full |
| Query Performance | ✅ Direct SQL | ✅ REST + SQL |
| Row Level Security | Manual | Built-in |
| Cost | Infrastructure | Usage-based |
| Scaling | Manual | Automatic |

## Common DataStore Interface

Both implementations use the same `DataStore` interface, making it easy to switch between them:

```go
type DataStore interface {
    Collection(name string) CollectionRef
    Transaction(ctx context.Context, fn func(tx Transaction) error) error
    Close() error
}
```

### CRUD Operations

Both clients support the same operations:

```go
// Insert
id, err := collection.Insert(ctx, data)

// Get
doc, err := collection.Get(ctx, id)

// Update
err := collection.Update(ctx, id, updateData)

// Delete
err := collection.Delete(ctx, id)

// Query
docs, err := collection.Query(ctx, filter, options...)
```

### Transactions

Both clients support transactions:

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    collection := tx.Collection("table_name")

    // Perform multiple operations
    id, err := collection.Insert(ctx, data)
    if err != nil {
        return err // Automatic rollback
    }

    err = collection.Update(ctx, id, updateData)
    if err != nil {
        return err // Automatic rollback
    }

    return nil // Commit
})
```

### Multi-Tenancy

Both clients enforce organization-level isolation:

```go
// Set organization ID in context
ctx := multitenancy.WithOrgID(context.Background(), "org-id")

// All operations are automatically scoped to this organization
collection := client.Collection("users")
docs, err := collection.Query(ctx, filter) // Only returns docs for "org-id"
```

## Database Schema Requirements

Both implementations require tables with these fields:

```sql
CREATE TABLE table_name (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL,
    -- Your custom fields here --
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP
);

CREATE INDEX idx_table_org_id ON table_name(org_id);
```

## Migration Between DataStores

To migrate from one datastore to another:

1. **Keep the same table schema**
2. **Update initialization code**:

```go
// From PostgreSQL
client, err := postgres.New(connectionString)

// To Supabase
client, err := supabase.New(url, apiKey, supabase.WithDB(db))
```

3. **No code changes needed** for CRUD operations (same interface)

## Best Practices

### 1. Connection Management

```go
// Always close the client when done
defer client.Close()

// Use connection pooling in production
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### 2. Error Handling

```go
doc, err := collection.Get(ctx, id)
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Handle not found
    } else {
        // Handle other errors
        log.Printf("Database error: %v", err)
    }
}
```

### 3. Context Usage

```go
// Use context for timeouts
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

ctx = multitenancy.WithOrgID(ctx, orgID)
```

### 4. Indexing

```sql
-- Add indexes for frequently queried fields
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_status ON users(status);
CREATE INDEX idx_users_created_at ON users(created_at DESC);
```

### 5. Pagination

```go
// Use limit and offset for pagination
page := 1
pageSize := 20

docs, err := collection.Query(ctx,
    filter,
    interfaces.QueryWithLimit(pageSize),
    interfaces.QueryWithOffset((page - 1) * pageSize),
    interfaces.QueryWithOrderBy("created_at", "desc"),
)
```

## Testing

Both examples include comprehensive tests:

```bash
# Test PostgreSQL client
cd postgres
export POSTGRES_URL="postgres://..."
go test

# Test Supabase client
cd supabase
export SUPABASE_URL="https://..."
export SUPABASE_API_KEY="..."
export SUPABASE_DB_URL="postgresql://..."
go test
```

## Performance Tips

1. **Use indexes** on frequently queried fields
2. **Batch operations** when possible
3. **Use connection pooling** for high-traffic applications
4. **Cache frequently accessed data**
5. **Monitor query performance** and optimize slow queries
6. **Use transactions** only when needed (overhead)

## Security Considerations

1. **Environment Variables**: Store credentials in env vars, never in code
2. **Least Privilege**: Use database users with minimal required permissions
3. **Input Validation**: Validate all data before database operations
4. **SQL Injection**: Both clients use parameterized queries (safe)
5. **Multi-Tenancy**: Always set organization ID in context
6. **Connection Security**: Use SSL/TLS in production

## Additional Resources

- [DataStore Interface Documentation](../../pkg/interfaces/datastore.go)
- [PostgreSQL Client Documentation](../../pkg/datastore/postgres/README.md)
- [Supabase Client Documentation](../../pkg/datastore/supabase/supabase.go)
- [Multi-Tenancy Package](../../pkg/multitenancy/)

## Contributing

To add a new DataStore implementation:

1. Implement the `DataStore` interface
2. Add CRUD methods to your collection type
3. Implement transaction support
4. Add tests
5. Create an example
6. Update this README

## Troubleshooting

### SSL Connection Error

**Error:** `pq: SSL is not enabled on the server`

**Solution:** Add `?sslmode=disable` to your connection string:

```bash
# Before (causes error)
export POSTGRES_URL="postgres://user:pass@localhost:5432/db"

# After (fixed)
export POSTGRES_URL="postgres://user:pass@localhost:5432/db?sslmode=disable"
```

### Connection Refused

**Error:** `connection refused`

**Solution:**
- Verify PostgreSQL is running: `ps aux | grep postgres`
- Check the host and port are correct
- Ensure firewall allows connections

### Authentication Failed

**Error:** `password authentication failed`

**Solution:**
- Verify username and password are correct
- Check PostgreSQL `pg_hba.conf` allows your connection method
- Ensure the user has proper permissions

### Database Does Not Exist

**Error:** `database "dbname" does not exist`

**Solution:**
```bash
# Create the database
createdb dbname

# Or via SQL
psql -c "CREATE DATABASE dbname;"
```

## Support

For issues or questions:

- Check the individual client READMEs
- Review the troubleshooting section above
- Review the interface documentation
- See the test files for usage examples
- Open an issue in the repository
