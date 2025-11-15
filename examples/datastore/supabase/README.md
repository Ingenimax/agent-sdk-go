# Supabase DataStore Example

This example demonstrates how to use the Supabase DataStore client with the Agent SDK.

## Features Demonstrated

- Creating a Supabase client with REST API and database connection
- Inserting documents via Supabase REST API
- Retrieving documents by ID
- Updating documents
- Querying documents with filters
- Querying with limit and ordering
- Using transactions for atomic operations (via direct SQL)
- Multi-tenancy with organization ID isolation

## Prerequisites

1. Supabase project created
2. Supabase API credentials (URL and API Key)
3. Database connection string for transactions
4. Test table created (see schema below)

## Database Setup

Create a test table in your Supabase project (SQL Editor):

```sql
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    age INTEGER,
    status TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP
);

CREATE INDEX idx_users_org_id ON users(org_id);
CREATE INDEX idx_users_status ON users(status);
```

## Configuration

Set the required environment variables:

```bash
export SUPABASE_URL="https://your-project.supabase.co"
export SUPABASE_API_KEY="your-anon-or-service-role-key"
export SUPABASE_DB_URL="postgresql://postgres:[YOUR-PASSWORD]@db.your-project.supabase.co:5432/postgres"
```

### Getting Your Credentials

1. **SUPABASE_URL**: Found in your Supabase project settings under "API"
2. **SUPABASE_API_KEY**: Use either:
   - `anon` key for client-side applications
   - `service_role` key for server-side applications (more permissions)
3. **SUPABASE_DB_URL**: Found in project settings under "Database" → "Connection string" → "URI"

## Running the Example

```bash
cd examples/datastore/supabase
go run main.go
```

## Expected Output

The example will:

1. Insert a new user document
2. Retrieve the document by ID
3. Update the document
4. Retrieve the updated document
5. Insert multiple user documents
6. Query active users
7. Query with limit and ordering
8. Perform a transaction with multiple operations
9. Clean up by deleting all created documents

## Example Output

```
Supabase DataStore Example
==========================

1. Inserting a document...
   Inserted user with ID: 123e4567-e89b-12d3-a456-426614174000

2. Retrieving the document...
   Retrieved user: map[age:28 created_at:2024-01-15T10:30:00Z email:alice@example.com id:123e4567-e89b-12d3-a456-426614174000 name:Alice Johnson org_id:demo-org-123 status:active]

3. Updating the document...
   Document updated successfully

4. Retrieving updated document...
   Updated user: map[age:29 created_at:2024-01-15T10:30:00Z email:alice@example.com id:123e4567-e89b-12d3-a456-426614174000 name:Alice Johnson org_id:demo-org-123 status:verified updated_at:2024-01-15T10:30:15Z]

5. Inserting multiple documents...
   Inserted user: Bob Smith with ID: 223e4567-e89b-12d3-a456-426614174001
   Inserted user: Carol White with ID: 323e4567-e89b-12d3-a456-426614174002
   Inserted user: David Brown with ID: 423e4567-e89b-12d3-a456-426614174003

6. Querying active users...
   Found 3 active users
   - Alice Johnson (alice@example.com)
   - Bob Smith (bob@example.com)
   - Carol White (carol@example.com)

7. Querying with limit and ordering...
   Retrieved top 2 active users (ordered by name):
   - Alice Johnson
   - Bob Smith

8. Using transactions...
   Transaction completed successfully

9. Cleaning up - deleting all created documents...
   Deleted user with ID: 123e4567-e89b-12d3-a456-426614174000
   Deleted user with ID: 223e4567-e89b-12d3-a456-426614174001
   Deleted user with ID: 323e4567-e89b-12d3-a456-426614174002
   Deleted user with ID: 423e4567-e89b-12d3-a456-426614174003
   Deleted user with ID: 523e4567-e89b-12d3-a456-426614174004

Example completed successfully!
```

## Key Concepts

### Supabase Architecture

The Supabase client uses two connection methods:

1. **REST API**: For most CRUD operations (Insert, Get, Update, Delete, Query)
2. **Direct SQL**: For transactions that require atomicity

### Multi-Tenancy

All operations are automatically scoped to an organization ID:

```go
ctx := multitenancy.WithOrgID(context.Background(), "demo-org-123")
```

This ensures data isolation between different organizations.

### Automatic Fields

The client automatically manages these fields:

- `id`: Generated UUID if not provided
- `org_id`: Set from context
- `created_at`: Set on insert
- `updated_at`: Set on update

### Row Level Security (RLS)

For production use, consider enabling Row Level Security in Supabase:

```sql
-- Enable RLS
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Create policy for multi-tenancy
CREATE POLICY "Users can only access their org data"
ON users
FOR ALL
USING (org_id = current_setting('app.current_org_id')::text);
```

### Transactions

Transactions use direct SQL connection for atomicity:

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    collection := tx.Collection("users")

    // Multiple operations...
    // If any fails, all are rolled back

    return nil // or return error to rollback
})
```

### Query Options

Available query options:

- `interfaces.QueryWithLimit(n)`: Limit results
- `interfaces.QueryWithOffset(n)`: Skip results (not supported via REST API)
- `interfaces.QueryWithOrderBy(field, direction)`: Order results

## Differences from PostgreSQL Client

The Supabase client differs from the pure PostgreSQL client:

1. **REST API First**: Uses Supabase REST API for most operations
2. **Additional Features**: Can leverage Supabase features like:
   - Real-time subscriptions
   - Built-in authentication
   - Row Level Security
   - Storage integration
3. **Transaction Support**: Requires database connection string for transactions

## Error Handling

The example includes basic error handling. In production, you should:

- Check for specific error types
- Implement retry logic for transient failures
- Log errors appropriately
- Handle both REST API and database connection issues
- Monitor Supabase quotas and limits

## Performance Considerations

- **REST API**: Optimized for most CRUD operations
- **Batching**: Consider batching inserts for better performance
- **Indexes**: Add indexes on frequently queried fields
- **Connection Pooling**: Use connection pooling for database connections
- **Caching**: Consider caching frequently accessed data

## Security Best Practices

1. Use service role key only on server-side
2. Enable Row Level Security (RLS)
3. Validate and sanitize all input data
4. Use environment variables for credentials
5. Implement proper authentication and authorization

## Next Steps

- Explore Supabase real-time features
- Implement Row Level Security policies
- Add authentication integration
- Use Supabase storage for file uploads
- Implement data validation and schemas

## Related Examples

- `/examples/datastore/postgres` - Pure PostgreSQL DataStore example
- `/examples/memory` - Memory storage example

## Documentation

- [Supabase DataStore Documentation](../../../pkg/datastore/supabase/supabase.go)
- [DataStore Interface](../../../pkg/interfaces/datastore.go)
- [Supabase Documentation](https://supabase.com/docs)

## Troubleshooting

### Connection Issues

If you get connection errors:

1. Verify your Supabase URL and API key
2. Check if your IP is allowed (Supabase dashboard → Settings → Database)
3. Ensure the database is not paused

### Transaction Failures

If transactions fail:

1. Verify SUPABASE_DB_URL is correct
2. Ensure you have database access enabled
3. Check connection pooling settings

### Query Limitations

Note: Offset is not supported via the REST API. Use cursor-based pagination for large datasets.
