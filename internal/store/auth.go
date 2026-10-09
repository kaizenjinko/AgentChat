package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	PasswordHash       string `json:"-"`
	Role               string `json:"role"`
	Status             string `json:"status"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          string `json:"created_at"`
	LastLoginAt        string `json:"last_login_at"`
}

type APIKey struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	AgentName  string `json:"agent_name"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

func HashPassword(pw string, cost int) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

var (
	dummyOnce sync.Once
	dummyHash string
)

func DummyHash() string {
	dummyOnce.Do(func() {
		h, err := HashPassword("agentchat-dummy-password", 12)
		if err != nil {
			h = "$2a$12$C6UzMDM.H6dfI/f/IKcEeO7ZKXj2wFsZbYpGg/tYq5LwVqvzXqsS6"
		}
		dummyHash = h
	})
	return dummyHash
}

func nowUTC() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func scanUser(sc scanner) (*User, error) {
	var (
		u        User
		mustCh   int
		created  sql.NullString
		lastSeen sql.NullString
	)
	if err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Status, &mustCh, &created, &lastSeen); err != nil {
		return nil, err
	}
	u.MustChangePassword = mustCh != 0
	u.CreatedAt = created.String
	u.LastLoginAt = lastSeen.String
	return &u, nil
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, password_hash, role, status, must_change_password, created_at, last_login_at
		 FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (s *Store) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, password_hash, role, status, must_change_password, created_at, last_login_at
		 FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) SeedAdmin() error {
	_, err := s.GetUserByUsername("admin")
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	hash, err := HashPassword("123456", 12)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO users (username, password_hash, role, status, must_change_password)
		 VALUES ('admin', ?, 'admin', 'active', 1)`, hash)
	return err
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) CreateSession(userID int64, ttl time.Duration) (string, error) {
	id, err := randomHex(32)
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(ttl).Format("2006-01-02 15:04:05")
	_, err = s.db.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		id, userID, expires)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) GetSession(id string) (*User, error) {
	row := s.db.QueryRow(
		`SELECT u.id, u.username, u.password_hash, u.role, u.status, u.must_change_password, u.created_at, u.last_login_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.id = ? AND s.expires_at > ? AND u.status = 'active'`,
		id, nowUTC())
	u, err := scanUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return u, nil
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteSessionsForUser(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) TouchLastLogin(userID int64) error {
	_, err := s.db.Exec(`UPDATE users SET last_login_at=datetime('now') WHERE id=?`, userID)
	return err
}

func hashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateAPIKey(userID int64, name, agentName string) (plain string, prefix string, err error) {
	raw, err := randomHex(32)
	if err != nil {
		return "", "", err
	}
	plain = "ac_" + raw
	keyHash := hashKey(plain)
	if len(plain) < 12 {
		prefix = plain
	} else {
		prefix = plain[:12]
	}
	var agent any
	if agentName != "" {
		agent = agentName
	}
	_, err = s.db.Exec(
		`INSERT INTO api_keys (user_id, name, key_hash, prefix, agent_name, status)
		 VALUES (?, ?, ?, ?, ?, 'active')`,
		userID, name, keyHash, prefix, agent)
	if err != nil {
		return "", "", err
	}
	return plain, prefix, nil
}

func (s *Store) UserByAPIKey(plain string) (*User, error) {
	keyHash := hashKey(plain)
	var kID int64
	var lastUsed sql.NullString
	var u User
	var mustCh int
	var created, lastSeen sql.NullString
	err := s.db.QueryRow(
		`SELECT k.id, k.last_used_at, u.id, u.username, u.password_hash, u.role, u.status, u.must_change_password, u.created_at, u.last_login_at
		 FROM api_keys k JOIN users u ON u.id = k.user_id
		 WHERE k.key_hash = ? AND k.status = 'active' AND u.status = 'active'`,
		keyHash).Scan(&kID, &lastUsed, &u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Status, &mustCh, &created, &lastSeen)
	if err != nil {
		return nil, err
	}
	u.MustChangePassword = mustCh != 0
	u.CreatedAt = created.String
	u.LastLoginAt = lastSeen.String
	// Throttle last_used_at writes: an agent polling every few seconds would
	// otherwise force a WAL write per request. Only refresh when the stored
	// stamp is older than the window, cutting writes ~10x on the edge.
	if lastUsed.String == "" || lastUsedStale(lastUsed.String) {
		if _, err := s.db.Exec(`UPDATE api_keys SET last_used_at=datetime('now') WHERE id=?`, kID); err != nil {
			return nil, err
		}
	}
	return &u, nil
}

// lastUsedStale reports whether a stored "YYYY-MM-DD HH:MM:SS" UTC stamp is
// older than apiKeyTouchWindow.
func lastUsedStale(stamp string) bool {
	t, err := time.Parse("2006-01-02 15:04:05", stamp)
	if err != nil {
		return true
	}
	return time.Since(t.UTC()) >= apiKeyTouchWindow
}

// apiKeyTouchWindow bounds how often an API key's last_used_at is rewritten.
const apiKeyTouchWindow = 60 * time.Second

func (s *Store) ListAPIKeys(userID int64) ([]APIKey, error) {
	rows, err := s.db.Query(
		`SELECT id, name, prefix, agent_name, status, created_at, last_used_at
		 FROM api_keys WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]APIKey, 0)
	for rows.Next() {
		var (
			k        APIKey
			agent    sql.NullString
			created  sql.NullString
			lastUsed sql.NullString
		)
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &agent, &k.Status, &created, &lastUsed); err != nil {
			return nil, err
		}
		k.AgentName = agent.String
		k.CreatedAt = created.String
		k.LastUsedAt = lastUsed.String
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) RevokeAPIKey(userID, id int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
