# PostgreSQL DataStore Example

This example demonstrates how to use the PostgreSQL DataStore client with the Agent SDK.

## Features Demonstrated

- **Self-contained setup**: Automatically creates and drops the test table
- Creating a PostgreSQL client
- Creating tables with indexes
- Inserting documents
- Retrieving documents by ID
- Updating documents
- Querying documents with filters
- Querying with limit and ordering
- Using transactions for atomic operations
- Multi-tenancy with organization ID isolation
- Cleaning up resources

## Prerequisites

1. PostgreSQL server running and accessible
2. A database created with appropriate permissions
3. **No manual table setup required** - the example creates and drops the table automatically

## What the Example Does

The example automatically:

1. **Creates the `users` table** with proper schema and indexes
2. Runs all CRUD and query operations
3. **Drops the `users` table** at the end for clean up

This makes it completely self-contained and safe to run multiple times without manual cleanup.

## Configuration

Set the PostgreSQL connection string as an environment variable:

```bash
# Local development (SSL disabled)
export POSTGRES_URL="postgres://username:password@localhost:5432/dbname?sslmode=disable"

# Production (SSL enabled)
export POSTGRES_URL="postgres://username:password@host:5432/dbname?sslmode=require"
```

Replace the connection string with your actual PostgreSQL credentials:
- `username`: Your PostgreSQL username
- `password`: Your PostgreSQL password
- `localhost:5432`: Your PostgreSQL host and port
- `dbname`: Your database name
- `sslmode=disable`: SSL mode (see SSL Configuration below)

### SSL Configuration

If you get an error like "SSL is not enabled on the server", add `?sslmode=disable` to your connection string:

```bash
export POSTGRES_URL="postgres://user:password@localhost:5432/dbname?sslmode=disable"
```

Available SSL modes:
- `disable` - No SSL (local development, trusted networks)
- `require` - Require SSL but don't verify certificate
- `verify-ca` - Require SSL and verify CA certificate
- `verify-full` - Require SSL and verify CA + hostname (recommended for production)

## Running the Example

```bash
cd examples/datastore/postgres
go run main.go
```

## Expected Output

The example will:

1. Drop any existing `users` table (clean slate)
2. Create the `users` table with indexes
3. Insert a new user document
4. Retrieve the document by ID
5. Update the document
6. Retrieve the updated document
7. Insert multiple user documents
8. Query active users
9. Query with limit and ordering
10. Perform a transaction with multiple operations
11. Delete all created documents
12. Drop the `users` table

## Example Output

```
PostgreSQL DataStore Example
============================

Setting up database...
1. Cleaning up any existing tables...
   Cleaned up successfully
2. Creating users table...
   Table created successfully

3. Inserting a document...
   Inserted user with ID: 123e4567-e89b-12d3-a456-426614174000

4. Retrieving the document...
   Retrieved user: map[age:28 created_at:2024-01-15T10:30:00Z email:alice@example.com id:123e4567-e89b-12d3-a456-426614174000 name:Alice Johnson org_id:demo-org-123 status:active]

5. Updating the document...
   Document updated successfully

6. Retrieving updated document...
   Updated user: map[age:29 created_at:2024-01-15T10:30:00Z email:alice@example.com id:123e4567-e89b-12d3-a456-426614174000 name:Alice Johnson org_id:demo-org-123 status:verified updated_at:2024-01-15T10:30:15Z]

7. Inserting multiple documents...
   Inserted user: Bob Smith with ID: 223e4567-e89b-12d3-a456-426614174001
   Inserted user: Carol White with ID: 323e4567-e89b-12d3-a456-426614174002
   Inserted user: David Brown with ID: 423e4567-e89b-12d3-a456-426614174003

8. Querying active users...
   Found 3 active users
   - Alice Johnson (alice@example.com)
   - Bob Smith (bob@example.com)
   - Carol White (carol@example.com)

9. Querying with limit and ordering...
   Retrieved top 2 active users (ordered by name):
   - Alice Johnson
   - Bob Smith

10. Using transactions...
   Transaction completed successfully

11. Cleaning up - deleting all created documents...
   Deleted user with ID: 123e4567-e89b-12d3-a456-426614174000
   Deleted user with ID: 223e4567-e89b-12d3-a456-426614174001
   Deleted user with ID: 323e4567-e89b-12d3-a456-426614174002
   Deleted user with ID: 423e4567-e89b-12d3-a456-426614174003
   Deleted user with ID: 523e4567-e89b-12d3-a456-426614174004

12. Dropping users table...
    Table dropped successfully

Example completed successfully!
```

## Key Concepts

### Self-Contained Example

This example is completely self-contained and:

- **Creates its own table** with the proper schema and indexes
- Runs all operations on the created table
- **Cleans up after itself** by dropping the table at the end
- Can be run multiple times without conflicts
- Requires no manual database setup beyond creating the database itself

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

### Transactions

Transactions ensure atomicity of multiple operations:

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
- `interfaces.QueryWithOffset(n)`: Skip results
- `interfaces.QueryWithOrderBy(field, direction)`: Order results

## Error Handling

The example includes basic error handling. In production, you should:

- Check for specific error types
- Implement retry logic for transient failures
- Log errors appropriately
- Handle database connection issues

## Next Steps

- Explore more complex queries
- Implement pagination for large datasets
- Add indexes for better performance
- Use connection pooling for production workloads
- Implement proper error handling and logging

## Related Examples

- `/examples/datastore/supabase` - Supabase DataStore example
- `/examples/memory` - Memory storage example

## Documentation

- [PostgreSQL DataStore Documentation](../../../pkg/datastore/postgres/README.md)
- [DataStore Interface](../../../pkg/interfaces/datastore.go)
