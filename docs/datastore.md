# DataStore

This document explains how to use the DataStore component of the Agent SDK.

## Overview

DataStores provide a unified interface for storing and retrieving structured data with built-in multi-tenancy support. They enable CRUD operations, querying, and transactional updates across different database backends.

## Key Features

- **Unified Interface**: Same API across different database implementations
- **Multi-Tenancy**: Built-in organization-level data isolation
- **CRUD Operations**: Create, Read, Update, Delete with automatic ID generation
- **Query Support**: Filter, limit, offset, and ordering capabilities
- **Transactions**: Atomic operations with rollback support
- **Automatic Timestamps**: Managed `created_at` and `updated_at` fields

## Supported DataStores

The SDK currently supports two DataStore implementations:

1. **PostgreSQL** - Direct PostgreSQL database connection
2. **Supabase** - REST API + PostgreSQL for enhanced features

## PostgreSQL DataStore

### Installation

```bash
go get github.com/lib/pq
```

### Basic Setup

```go
import (
    "context"
    "github.com/Ingenimax/agent-sdk-go/pkg/datastore/postgres"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Create a PostgreSQL client
client, err := postgres.New("postgres://user:password@localhost:5432/dbname?sslmode=disable")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Set organization context for multi-tenancy
ctx := multitenancy.WithOrgID(context.Background(), "org-123")

// Get a collection reference
collection := client.Collection("users")
```

### Using an Existing Database Connection

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
)

// Create database connection
db, err := sql.Open("postgres", connectionString)
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

### Connection String Format

```
postgres://username:password@host:port/database?sslmode=MODE
```

**SSL Modes:**
- `disable` - No SSL (local development only)
- `require` - Require SSL but don't verify certificate
- `verify-ca` - Require SSL and verify certificate authority
- `verify-full` - Require SSL and verify certificate + hostname (recommended for production)

**Examples:**

```bash
# Local development
export POSTGRES_URL="postgres://postgres:password@localhost:5432/mydb?sslmode=disable"

# Production
export POSTGRES_URL="postgres://user:pass@prod-server:5432/mydb?sslmode=verify-full"

# Docker Compose
export POSTGRES_URL="postgres://user:password@postgres:5432/mydb?sslmode=disable"
```

## Supabase DataStore

### Installation

```bash
go get github.com/supabase-community/supabase-go
go get github.com/lib/pq
```

### Basic Setup

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
    "github.com/Ingenimax/agent-sdk-go/pkg/datastore/supabase"
    "github.com/Ingenimax/agent-sdk-go/pkg/multitenancy"
)

// Connect to database for transaction support
db, err := sql.Open("postgres", dbURL)
if err != nil {
    log.Fatal(err)
}

// Create Supabase client
client, err := supabase.New(
    "https://your-project.supabase.co",
    "your-api-key",
    supabase.WithDB(db),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Set organization context for multi-tenancy
ctx := multitenancy.WithOrgID(context.Background(), "org-123")

// Get a collection reference
collection := client.Collection("users")
```

### Environment Variables

```bash
export SUPABASE_URL="https://your-project.supabase.co"
export SUPABASE_API_KEY="your-anon-or-service-role-key"
export SUPABASE_DB_URL="postgresql://postgres:[PASSWORD]@db.your-project.supabase.co:5432/postgres"
```

## Database Schema Requirements

All tables used with DataStore must have the following columns:

```sql
CREATE TABLE table_name (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL,
    -- Your custom fields here --
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP
);

-- Required indexes
CREATE INDEX idx_table_org_id ON table_name(org_id);

-- Recommended indexes for frequently queried fields
CREATE INDEX idx_table_status ON table_name(status);
CREATE INDEX idx_table_created_at ON table_name(created_at DESC);
```

### Field Descriptions

- `id` (TEXT/UUID): Primary key, auto-generated if not provided
- `org_id` (TEXT): Organization identifier for multi-tenancy isolation
- `created_at` (TIMESTAMP): Automatically set on insert
- `updated_at` (TIMESTAMP): Automatically set on update
- Custom fields: Add any additional fields your application needs

## CRUD Operations

### Insert

Insert a new document into a collection:

```go
data := map[string]interface{}{
    "name":  "John Doe",
    "email": "john@example.com",
    "age":   30,
    "status": "active",
}

// ID is auto-generated if not provided
id, err := collection.Insert(ctx, data)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Inserted document with ID: %s\n", id)
```

**With Custom ID:**

```go
data := map[string]interface{}{
    "id":    "custom-id-123",
    "name":  "Jane Doe",
    "email": "jane@example.com",
}

id, err := collection.Insert(ctx, data)
```

### Get

Retrieve a document by ID:

```go
doc, err := collection.Get(ctx, id)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Name: %s\n", doc["name"])
fmt.Printf("Email: %s\n", doc["email"])
```

### Update

Update specific fields of a document:

```go
updateData := map[string]interface{}{
    "age":    31,
    "status": "verified",
}

err = collection.Update(ctx, id, updateData)
if err != nil {
    log.Fatal(err)
}
```

### Delete

Delete a document by ID:

```go
err = collection.Delete(ctx, id)
if err != nil {
    log.Fatal(err)
}
```

## Query Operations

### Basic Query

Query documents with filters:

```go
docs, err := collection.Query(ctx, map[string]interface{}{
    "status": "active",
})
if err != nil {
    log.Fatal(err)
}

for _, doc := range docs {
    fmt.Printf("User: %s (%s)\n", doc["name"], doc["email"])
}
```

### Query with Limit

```go
import "github.com/Ingenimax/agent-sdk-go/pkg/interfaces"

docs, err := collection.Query(ctx,
    map[string]interface{}{"status": "active"},
    interfaces.QueryWithLimit(10),
)
```

### Query with Pagination

```go
page := 2
pageSize := 20

docs, err := collection.Query(ctx,
    map[string]interface{}{"status": "active"},
    interfaces.QueryWithLimit(pageSize),
    interfaces.QueryWithOffset((page - 1) * pageSize),
)
```

### Query with Ordering

```go
// Ascending order
docs, err := collection.Query(ctx,
    map[string]interface{}{"status": "active"},
    interfaces.QueryWithOrderBy("created_at", "asc"),
)

// Descending order
docs, err := collection.Query(ctx,
    map[string]interface{}{"status": "active"},
    interfaces.QueryWithOrderBy("created_at", "desc"),
)
```

### Complex Query

Combine multiple options:

```go
docs, err := collection.Query(ctx,
    map[string]interface{}{
        "status": "active",
        "type":   "premium",
    },
    interfaces.QueryWithOrderBy("created_at", "desc"),
    interfaces.QueryWithLimit(50),
    interfaces.QueryWithOffset(100),
)
```

## Transactions

Transactions ensure atomicity across multiple operations. If any operation fails, all changes are rolled back.

### Basic Transaction

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    collection := tx.Collection("users")

    // Insert user
    userID, err := collection.Insert(ctx, map[string]interface{}{
        "name":  "Alice",
        "email": "alice@example.com",
    })
    if err != nil {
        return err // Automatic rollback
    }

    // Update user
    err = collection.Update(ctx, userID, map[string]interface{}{
        "status": "active",
    })
    if err != nil {
        return err // Automatic rollback
    }

    return nil // Commit
})

if err != nil {
    log.Printf("Transaction failed: %v", err)
}
```

### Multiple Collections in Transaction

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    users := tx.Collection("users")
    orders := tx.Collection("orders")

    // Create user
    userID, err := users.Insert(ctx, map[string]interface{}{
        "name": "Bob",
    })
    if err != nil {
        return err
    }

    // Create order for user
    _, err = orders.Insert(ctx, map[string]interface{}{
        "user_id": userID,
        "amount":  99.99,
    })
    if err != nil {
        return err // Both operations will be rolled back
    }

    return nil
})
```

### Conditional Rollback

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    collection := tx.Collection("accounts")

    // Get account
    account, err := collection.Get(ctx, accountID)
    if err != nil {
        return err
    }

    // Check balance
    balance := account["balance"].(float64)
    if balance < 100 {
        return fmt.Errorf("insufficient balance")
    }

    // Deduct amount
    err = collection.Update(ctx, accountID, map[string]interface{}{
        "balance": balance - 100,
    })
    if err != nil {
        return err
    }

    return nil
})
```

## Multi-Tenancy

All DataStore operations are automatically scoped to an organization ID for data isolation.

### Setting Organization Context

```go
// Create context with organization ID
ctx := multitenancy.WithOrgID(context.Background(), "org-123")

// All operations on this context are scoped to "org-123"
collection := client.Collection("users")
docs, _ := collection.Query(ctx, map[string]interface{}{})
// Only returns documents where org_id = "org-123"
```

### Different Organizations

```go
// Organization A
ctxA := multitenancy.WithOrgID(context.Background(), "org-a")
collection.Insert(ctxA, map[string]interface{}{"name": "User A"})

// Organization B
ctxB := multitenancy.WithOrgID(context.Background(), "org-b")
collection.Insert(ctxB, map[string]interface{}{"name": "User B"})

// These documents are completely isolated from each other
```

### Missing Organization Context

If no organization ID is set in the context, operations will fail:

```go
ctx := context.Background() // No org ID
_, err := collection.Insert(ctx, data)
// Error: failed to get organization ID
```

## Error Handling

### Common Errors

```go
doc, err := collection.Get(ctx, id)
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Document doesn't exist or belongs to different org
        log.Println("Document not found")
    } else if strings.Contains(err.Error(), "connection refused") {
        // Database connection issue
        log.Println("Database unavailable")
    } else {
        // Other errors
        log.Printf("Database error: %v", err)
    }
}
```

### Update/Delete Errors

```go
err := collection.Update(ctx, id, updateData)
if err != nil {
    if strings.Contains(err.Error(), "not found") {
        // Document doesn't exist or belongs to different org
    } else if strings.Contains(err.Error(), "not owned by organization") {
        // Document exists but belongs to different organization
    }
}
```

### Transaction Errors

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    // ... operations ...
    return someError
})

if err != nil {
    // Transaction was rolled back
    log.Printf("Transaction failed and rolled back: %v", err)
}
```

## Best Practices

### 1. Connection Management

```go
// Initialize once
client, err := postgres.New(connectionString)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Configure connection pool
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### 2. Context with Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

ctx = multitenancy.WithOrgID(ctx, orgID)

doc, err := collection.Get(ctx, id)
```

### 3. Batch Operations

For inserting multiple documents, consider using transactions:

```go
err := client.Transaction(ctx, func(tx interfaces.Transaction) error {
    collection := tx.Collection("users")

    for _, user := range users {
        _, err := collection.Insert(ctx, user)
        if err != nil {
            return err
        }
    }

    return nil
})
```

### 4. Indexing

Create indexes for frequently queried fields:

```sql
-- Composite index for common queries
CREATE INDEX idx_users_org_status ON users(org_id, status);

-- Index for sorting
CREATE INDEX idx_users_created_at ON users(created_at DESC);

-- Partial index for active records
CREATE INDEX idx_active_users ON users(org_id) WHERE status = 'active';
```

### 5. Data Validation

Validate data before inserting:

```go
func validateUser(data map[string]interface{}) error {
    if name, ok := data["name"].(string); !ok || name == "" {
        return fmt.Errorf("name is required")
    }

    if email, ok := data["email"].(string); !ok || !isValidEmail(email) {
        return fmt.Errorf("valid email is required")
    }

    return nil
}

// Use validation
if err := validateUser(userData); err != nil {
    log.Fatal(err)
}

id, err := collection.Insert(ctx, userData)
```

### 6. Soft Deletes

Instead of hard deletes, consider soft deletes:

```go
// Soft delete
err := collection.Update(ctx, id, map[string]interface{}{
    "deleted_at": time.Now(),
    "status":     "deleted",
})

// Query only active records
docs, err := collection.Query(ctx, map[string]interface{}{
    "status": "active",
})
```

## PostgreSQL vs Supabase

### When to Use PostgreSQL

- Self-hosted infrastructure
- Full control over database configuration
- Existing PostgreSQL setup
- On-premise deployments
- Custom database optimizations needed

### When to Use Supabase

- Cloud-hosted solution
- Rapid development
- Built-in authentication needed
- Real-time features required
- Managed infrastructure preferred
- Row Level Security (RLS) needed

### Feature Comparison

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

## Performance Tips

### 1. Use Appropriate Data Types

```sql
-- Use specific types instead of TEXT
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    age INTEGER,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
```

### 2. Optimize Queries

```go
// Bad: Fetches all fields
docs, _ := collection.Query(ctx, filter)

// Better: Fetch only needed documents with limit
docs, _ := collection.Query(ctx,
    filter,
    interfaces.QueryWithLimit(100),
    interfaces.QueryWithOrderBy("created_at", "desc"),
)
```

### 3. Batch Processing

Process large datasets in batches:

```go
const batchSize = 1000
offset := 0

for {
    docs, err := collection.Query(ctx,
        filter,
        interfaces.QueryWithLimit(batchSize),
        interfaces.QueryWithOffset(offset),
    )
    if err != nil {
        log.Fatal(err)
    }

    if len(docs) == 0 {
        break // No more documents
    }

    // Process batch
    for _, doc := range docs {
        // ...
    }

    offset += batchSize
}
```

### 4. Connection Pooling

```go
db, _ := sql.Open("postgres", connectionString)

// Configure pool
db.SetMaxOpenConns(25)        // Max open connections
db.SetMaxIdleConns(5)         // Max idle connections
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(1 * time.Minute)
```

## Troubleshooting

### Connection Issues

**Error:** `connection refused`

```bash
# Check PostgreSQL is running
ps aux | grep postgres

# Test connection
psql -h localhost -U user -d dbname

# Check firewall rules
```

**Error:** `SSL is not enabled on the server`

```bash
# Add sslmode=disable for local development
export POSTGRES_URL="postgres://user:pass@localhost/db?sslmode=disable"
```

### Authentication Issues

**Error:** `password authentication failed`

```bash
# Check pg_hba.conf
sudo nano /etc/postgresql/*/main/pg_hba.conf

# Add/modify line
host    all    all    0.0.0.0/0    md5
```

### Query Performance

**Slow queries:**

```sql
-- Enable query logging
ALTER DATABASE dbname SET log_statement = 'all';
ALTER DATABASE dbname SET log_duration = on;

-- Analyze query performance
EXPLAIN ANALYZE SELECT * FROM users WHERE org_id = 'org-123' AND status = 'active';

-- Add missing indexes
CREATE INDEX idx_users_org_status ON users(org_id, status);
```

## Examples

Complete examples are available in:

- `/examples/datastore/postgres/` - PostgreSQL example
- `/examples/datastore/supabase/` - Supabase example

Run examples:

```bash
# PostgreSQL
export POSTGRES_URL="postgres://user:pass@localhost/db?sslmode=disable"
go run ./examples/datastore/postgres/main.go

# Supabase
export SUPABASE_URL="https://your-project.supabase.co"
export SUPABASE_API_KEY="your-key"
export SUPABASE_DB_URL="postgresql://..."
go run ./examples/datastore/supabase/main.go
```

## Additional Resources

- [DataStore Interface](../pkg/interfaces/datastore.go)
- [PostgreSQL Client](../pkg/datastore/postgres/)
- [Supabase Client](../pkg/datastore/supabase/)
- [Multi-Tenancy Documentation](./multitenancy.md)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Supabase Documentation](https://supabase.com/docs)
