package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SQLStore persists sessions in any database/sql backend.
//
// It targets Postgres and SQLite, which differ in placeholder syntax ($1 versus
// ?) and in upsert spelling. Those are the only dialect-specific details, so
// rather than a driver interface per backend the store carries a small Dialect
// -- the same tradeoff ADK's DatabaseSessionService makes with its
// dialect-conditional branches.
//
// State is stored in three tables so the "user:" and "app:" prefixes are
// genuinely shared rather than copied into every session row. "temp:" keys are
// dropped before any write.
type SQLStore struct {
	db      *sql.DB
	dialect Dialect
}

// Dialect names a SQL flavour.
type Dialect string

const (
	// Postgres uses $N placeholders and ON CONFLICT upserts.
	Postgres Dialect = "postgres"
	// SQLite uses ? placeholders and ON CONFLICT upserts.
	SQLite Dialect = "sqlite"
)

// NewSQLStore creates a session store over an existing *sql.DB.
//
// It takes a live handle rather than a DSN on purpose: a service that already
// has a pool -- pkg/datastore/postgres, for instance -- should not open a
// second one just for sessions.
func NewSQLStore(db *sql.DB, dialect Dialect) (*SQLStore, error) {
	if db == nil {
		return nil, fmt.Errorf("session: nil *sql.DB")
	}
	switch dialect {
	case Postgres, SQLite:
	default:
		return nil, fmt.Errorf("session: unsupported dialect %q", dialect)
	}
	return &SQLStore{db: db, dialect: dialect}, nil
}

// rebind rewrites a query written with ? placeholders into the dialect's form.
func (s *SQLStore) rebind(query string) string {
	if s.dialect != Postgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Migrate creates the tables this store needs, if they do not exist.
func (s *SQLStore) Migrate(ctx context.Context) error {
	timestamp := "TIMESTAMPTZ"
	if s.dialect == SQLite {
		timestamp = "DATETIME"
	}

	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS agent_sessions (
			org_id        TEXT NOT NULL,
			id            TEXT NOT NULL,
			user_id       TEXT NOT NULL DEFAULT '',
			app_name      TEXT NOT NULL DEFAULT '',
			title         TEXT NOT NULL DEFAULT '',
			state         TEXT NOT NULL DEFAULT '{}',
			message_count INTEGER NOT NULL DEFAULT 0,
			token_total   INTEGER NOT NULL DEFAULT 0,
			created_at    %s NOT NULL,
			updated_at    %s NOT NULL,
			PRIMARY KEY (org_id, id)
		)`, timestamp, timestamp),

		`CREATE TABLE IF NOT EXISTS agent_session_user_state (
			org_id  TEXT NOT NULL,
			user_id TEXT NOT NULL,
			state   TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (org_id, user_id)
		)`,

		`CREATE TABLE IF NOT EXISTS agent_session_app_state (
			app_name TEXT NOT NULL,
			state    TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (app_name)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_listing
			ON agent_sessions (org_id, user_id, updated_at)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("session migrate: %w", err)
		}
	}
	return nil
}

// Create implements Store.
func (s *SQLStore) Create(ctx context.Context, sess *Session) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("session requires an ID")
	}

	now := time.Now().UTC()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	sess.UpdatedAt = now

	private, user, app := splitState(sess.State)
	privateJSON, err := json.Marshal(private)
	if err != nil {
		return fmt.Errorf("session: encoding state: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, s.rebind(`
		INSERT INTO agent_sessions
			(org_id, id, user_id, app_name, title, state, message_count, token_total, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		sess.OrgID, sess.ID, sess.UserID, sess.AppName, sess.Title,
		string(privateJSON), sess.MessageCount, sess.TokenTotal,
		sess.CreatedAt.UTC(), sess.UpdatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("session create: %w", err)
	}

	if err := s.mergeSharedTx(ctx, tx, sess, user, app); err != nil {
		return err
	}
	return tx.Commit()
}

// Get implements Store.
func (s *SQLStore) Get(ctx context.Context, orgID, id string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, s.rebind(`
		SELECT user_id, app_name, title, state, message_count, token_total, created_at, updated_at
		FROM agent_sessions WHERE org_id = ? AND id = ?`), orgID, id)

	var (
		sess      Session
		stateJSON string
	)
	sess.ID = id
	sess.OrgID = orgID

	if err := row.Scan(&sess.UserID, &sess.AppName, &sess.Title, &stateJSON,
		&sess.MessageCount, &sess.TokenTotal, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("session get: %w", err)
	}

	private := map[string]any{}
	if err := json.Unmarshal([]byte(stateJSON), &private); err != nil {
		return nil, fmt.Errorf("session: decoding state: %w", err)
	}

	user, err := s.readSharedState(ctx, `SELECT state FROM agent_session_user_state WHERE org_id = ? AND user_id = ?`, orgID, sess.UserID)
	if err != nil {
		return nil, err
	}
	app, err := s.readSharedState(ctx, `SELECT state FROM agent_session_app_state WHERE app_name = ?`, sess.AppName)
	if err != nil {
		return nil, err
	}

	sess.State = mergeState(private, user, app)
	return &sess, nil
}

func (s *SQLStore) readSharedState(ctx context.Context, query string, args ...any) (map[string]any, error) {
	var stateJSON string
	err := s.db.QueryRowContext(ctx, s.rebind(query), args...).Scan(&stateJSON)
	if err == sql.ErrNoRows {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: reading shared state: %w", err)
	}

	state := map[string]any{}
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return nil, fmt.Errorf("session: decoding shared state: %w", err)
	}
	return state, nil
}

// Update implements Store.
func (s *SQLStore) Update(ctx context.Context, sess *Session) error {
	if sess == nil || sess.ID == "" {
		return fmt.Errorf("session requires an ID")
	}

	sess.UpdatedAt = time.Now().UTC()

	private, user, app := splitState(sess.State)
	privateJSON, err := json.Marshal(private)
	if err != nil {
		return fmt.Errorf("session: encoding state: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, s.rebind(`
		UPDATE agent_sessions
		SET user_id = ?, app_name = ?, title = ?, state = ?,
		    message_count = ?, token_total = ?, updated_at = ?
		WHERE org_id = ? AND id = ?`),
		sess.UserID, sess.AppName, sess.Title, string(privateJSON),
		sess.MessageCount, sess.TokenTotal, sess.UpdatedAt.UTC(),
		sess.OrgID, sess.ID,
	)
	if err != nil {
		return fmt.Errorf("session update: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, sess.ID)
	}

	if err := s.mergeSharedTx(ctx, tx, sess, user, app); err != nil {
		return err
	}
	return tx.Commit()
}

// mergeSharedTx folds user- and app-scoped keys into their shared rows.
//
// The read-modify-write happens inside the caller's transaction so two sessions
// writing different user-scoped keys concurrently cannot lose one of them.
func (s *SQLStore) mergeSharedTx(ctx context.Context, tx *sql.Tx, sess *Session, user, app map[string]any) error {
	if len(user) > 0 && sess.UserID != "" {
		merged, err := s.mergeIntoRowTx(ctx, tx,
			`SELECT state FROM agent_session_user_state WHERE org_id = ? AND user_id = ?`,
			user, sess.OrgID, sess.UserID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO agent_session_user_state (org_id, user_id, state) VALUES (?, ?, ?)
			ON CONFLICT (org_id, user_id) DO UPDATE SET state = EXCLUDED.state`),
			sess.OrgID, sess.UserID, merged); err != nil {
			return fmt.Errorf("session: writing user state: %w", err)
		}
	}

	if len(app) > 0 && sess.AppName != "" {
		merged, err := s.mergeIntoRowTx(ctx, tx,
			`SELECT state FROM agent_session_app_state WHERE app_name = ?`,
			app, sess.AppName)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.rebind(`
			INSERT INTO agent_session_app_state (app_name, state) VALUES (?, ?)
			ON CONFLICT (app_name) DO UPDATE SET state = EXCLUDED.state`),
			sess.AppName, merged); err != nil {
			return fmt.Errorf("session: writing app state: %w", err)
		}
	}

	return nil
}

func (s *SQLStore) mergeIntoRowTx(ctx context.Context, tx *sql.Tx, query string, incoming map[string]any, args ...any) (string, error) {
	existing := map[string]any{}

	var stateJSON string
	err := tx.QueryRowContext(ctx, s.rebind(query), args...).Scan(&stateJSON)
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("session: reading shared state: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal([]byte(stateJSON), &existing); err != nil {
			return "", fmt.Errorf("session: decoding shared state: %w", err)
		}
	}

	for k, v := range incoming {
		existing[k] = v
	}

	encoded, err := json.Marshal(existing)
	if err != nil {
		return "", fmt.Errorf("session: encoding shared state: %w", err)
	}
	return string(encoded), nil
}

// Delete implements Store.
func (s *SQLStore) Delete(ctx context.Context, orgID, id string) error {
	// Only the session row goes. User- and app-scoped state is shared with
	// other sessions, so deleting one must not take it with them.
	result, err := s.db.ExecContext(ctx, s.rebind(
		`DELETE FROM agent_sessions WHERE org_id = ? AND id = ?`), orgID, id)
	if err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}

// List implements Store.
func (s *SQLStore) List(ctx context.Context, filter Filter) ([]*Session, error) {
	query := `SELECT org_id, id, user_id, app_name, title, message_count, token_total, created_at, updated_at
	          FROM agent_sessions`
	var conditions []string
	var args []any

	if filter.OrgID != "" {
		conditions = append(conditions, "org_id = ?")
		args = append(args, filter.OrgID)
	}
	if filter.UserID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, filter.UserID)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY updated_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, s.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("session list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.OrgID, &sess.ID, &sess.UserID, &sess.AppName,
			&sess.Title, &sess.MessageCount, &sess.TokenTotal,
			&sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, fmt.Errorf("session list: %w", err)
		}
		out = append(out, &sess)
	}
	return out, rows.Err()
}

var _ Store = (*SQLStore)(nil)
