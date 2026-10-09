package store

import "fmt"

// Migrate creates the auth tables and adds the messages.user_id column.
// It is idempotent: safe to run on every startup.
func (s *Store) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL DEFAULT 'user',
			status        TEXT NOT NULL DEFAULT 'active',
			must_change_password INTEGER NOT NULL DEFAULT 0,
			created_at    TEXT DEFAULT (datetime('now')),
			last_login_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name         TEXT NOT NULL DEFAULT '',
			key_hash     TEXT UNIQUE NOT NULL,
			prefix       TEXT NOT NULL,
			agent_name   TEXT,
			status       TEXT NOT NULL DEFAULT 'active',
			created_at   TEXT DEFAULT (datetime('now')),
			last_used_at TEXT
		)`,
		"CREATE INDEX IF NOT EXISTS idx_keys_user ON api_keys(user_id)",
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at TEXT DEFAULT (datetime('now')),
			expires_at TEXT NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)",
		`CREATE TABLE IF NOT EXISTS agents (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(user_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS rooms (
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now')),
			PRIMARY KEY (user_id, name)
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate %q: %w", q, err)
		}
	}

	hasUserID, err := s.hasColumn("messages", "user_id")
	if err != nil {
		return err
	}
	if !hasUserID {
		if _, err := s.db.Exec("ALTER TABLE messages ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0"); err != nil {
			return fmt.Errorf("migrate add messages.user_id: %w", err)
		}
	}

	if _, err := s.db.Exec("CREATE INDEX IF NOT EXISTS idx_user_room_id ON messages(user_id, room, id)"); err != nil {
		return fmt.Errorf("migrate idx_user_room_id: %w", err)
	}

	if _, err := s.db.Exec("UPDATE messages SET user_id=1 WHERE user_id=0"); err != nil {
		return fmt.Errorf("migrate backfill messages.user_id: %w", err)
	}

	if _, err := s.db.Exec("INSERT OR IGNORE INTO rooms (user_id, name) SELECT DISTINCT user_id, room FROM messages"); err != nil {
		return fmt.Errorf("migrate backfill rooms: %w", err)
	}

	return nil
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
