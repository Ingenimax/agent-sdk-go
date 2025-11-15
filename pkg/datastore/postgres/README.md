# PostgreSQL DataStore Client

A PostgreSQL implementation of the DataStore interface for the Agent SDK.

## Features

- Full CRUD operations (Create, Read, Update, Delete)
- Query support with filtering, ordering, limit, and offset
- Multi-tenancy support with organization ID isolation
- Transaction support for atomic operations
- Automatic ID generation and timestamp management

## Installation

```bash
go get github.com/lib/pq
```

## Usage

### Basic Setup

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/datastore/postgres"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create a new PostgreSQL client
client, err := postgres.New("postgres://user:password@localhost:5432/dbname?sslmode=disable")
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### Using an Existing Database Connection

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
)

// Create database connection
db, err := sql.Open("postgres", "postgres://user:password@localhost:5432/dbname?sslmode=disable")
if err != nil {
    log.Fatal(err)
}

// Create client with existing connection
client, err := postgres.NewWithDB(db)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

### CRUD Operations

```go
// Create a context with organization ID
ctx := multitenancy.WithOrgID(context.Background(), "org-123")

// Get a collection reference
collection := client.Collection("users")

// Insert a document
data := map[string]interface{}{
    "name":  "John Doe",
    "email": "john@example.com",
    "age":   30,
}
id, err := collection.Insert(ctx, data)

// Get a document by ID
doc, err := collection.Get(ctx, id)

// Update a document
updateData := map[string]interface{}{
    "age": 31,
}
err = collection.Update(ctx, id, updateData)

// Delete a document
err = collection.Delete(ctx, id)
```

### Query Operations

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

// Query with filter
docs, err := collection.Query(ctx, map[string]interface{}{
    "status": "active",
})

// Query with limit and offset
docs, err := collection.Query(ctx,
    map[string]interface{}{"status": "active"},
    interfaces.QueryWithLimit(10),
    interfaces.QueryWithOffset(20),
)

// Query with ordering
docs, err := collection.Query(ctx,
    map[string]interface{}{"status": "active"},
    interfaces.QueryWithOrderBy("created_at", "desc"),
    interfaces.QueryWithLimit(10),
)
```

### Transactions

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    collection := tx.Collection("users")

    // Insert document in transaction
    id1, err := collection.Insert(ctx, map[string]interface{}{
        "name": "Alice",
    })
    if err != nil {
        return err
    }

    // Update document in transaction
    err = collection.Update(ctx, id1, map[string]interface{}{
        "status": "active",
    })
    if err != nil {
        return err
    }

    // If any operation fails, the entire transaction will be rolled back
    return nil
})
```

## Database Schema Requirements

All tables should have the following columns:

- `id` (TEXT or UUID): Primary key
- `org_id` (TEXT): Organization ID for multi-tenancy
- `created_at` (TIMESTAMP): Creation timestamp (automatically set)
- `updated_at` (TIMESTAMP): Last update timestamp (automatically set on update)

Example table creation:

```sql
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    age INTEGER,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP,
    INDEX idx_org_id (org_id)
);
```

## Environment Variables

For testing, set the following environment variable:

```bash
# Local development (SSL disabled)
export POSTGRES_URL="postgres://user:password@localhost:5432/dbname?sslmode=disable"

# Production (SSL enabled)
export POSTGRES_URL="postgres://user:password@host:5432/dbname?sslmode=require"
```

### SSL Configuration

PostgreSQL SSL modes:
- `sslmode=disable` - No SSL (local development only)
- `sslmode=require` - Require SSL but don't verify certificate
- `sslmode=verify-ca` - Require SSL and verify certificate authority
- `sslmode=verify-full` - Require SSL and verify certificate + hostname (recommended for production)

## Multi-Tenancy

All operations automatically filter by organization ID from the context. To set the organization ID:

```go
ctx := multitenancy.WithOrgID(context.Background(), "your-org-id")
```

## Error Handling

The client returns descriptive errors for common scenarios:

- Document not found
- Document not owned by organization
- Database connection errors
- Query execution errors

Always check for errors and handle them appropriately:

```go
doc, err := collection.Get(ctx, id)
if err != nil {
    if strings.Contains(err.Error(), "document not found") {
        // Handle not found case
    } else {
        // Handle other errors
    }
}
```

## Testing

Run the tests with:

```bash
export POSTGRES_URL="postgres://user:password@localhost:5432/testdb?sslmode=disable"
go test ./pkg/datastore/postgres/...
```

Make sure you have a PostgreSQL database running and accessible with the connection string provided.
