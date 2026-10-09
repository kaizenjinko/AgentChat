package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

type UserWithStats struct {
	User
	RoomCount  int64 `json:"room_count"`
	MsgCount   int64 `json:"msg_count"`
	KeyCount   int64 `json:"key_count"`
	AgentCount int64 `json:"agent_count"`
}

func (s *Store) CountAdmins() (int64, error) {
	var n int64
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE role='admin' AND status='active'`).Scan(&n)
	return n, err
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

// ValidateUsername enforces a safe username charset and length.
func ValidateUsername(username string) error {
	if !usernameRe.MatchString(username) {
		return errors.New("username must be 3-32 chars of letters, digits, dot, underscore or hyphen")
	}
	return nil
}

func (s *Store) CreateUser(username, password, role string) (*User, error) {
	if role != "admin" && role != "user" {
		return nil, fmt.Errorf("invalid role %q", role)
	}
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	hash, err := HashPassword(password, 12)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role, status, must_change_password)
		 VALUES (?, ?, ?, 'active', 0)`,
		username, hash, role)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetUserByID(id)
}

func (s *Store) ListUsersWithStats() ([]UserWithStats, error) {
	rows, err := s.db.Query(
		`SELECT u.id, u.username, u.password_hash, u.role, u.status, u.must_change_password,
		        u.created_at, u.last_login_at,
		        (SELECT COUNT(DISTINCT room) FROM messages WHERE user_id=u.id) AS room_count,
		        (SELECT COUNT(*) FROM messages WHERE user_id=u.id) AS msg_count,
		        (SELECT COUNT(*) FROM api_keys WHERE user_id=u.id AND status='active') AS key_count,
		        (SELECT COUNT(*) FROM agents WHERE user_id=u.id) AS agent_count
		 FROM users u ORDER BY u.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]UserWithStats, 0)
	for rows.Next() {
		var (
			u        UserWithStats
			mustCh   int
			created  sql.NullString
			lastSeen sql.NullString
		)
		if err := rows.Scan(
			&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Status, &mustCh,
			&created, &lastSeen,
			&u.RoomCount, &u.MsgCount, &u.KeyCount, &u.AgentCount,
		); err != nil {
			return nil, err
		}
		u.MustChangePassword = mustCh != 0
		u.CreatedAt = created.String
		u.LastLoginAt = lastSeen.String
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) SetUserStatus(id int64, status string) error {
	if status != "active" && status != "disabled" {
		return fmt.Errorf("invalid status %q", status)
	}
	if status == "disabled" {
		u, err := s.GetUserByID(id)
		if err != nil {
			return err
		}
		if u.Role == "admin" && u.Status == "active" {
			n, err := s.CountAdmins()
			if err != nil {
				return err
			}
			if n <= 1 {
				return errors.New("cannot disable last admin")
			}
		}
	}
	_, err := s.db.Exec(`UPDATE users SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) SetUserRole(id int64, role string) error {
	if role != "admin" && role != "user" {
		return fmt.Errorf("invalid role %q", role)
	}
	if role != "admin" {
		u, err := s.GetUserByID(id)
		if err != nil {
			return err
		}
		if u.Role == "admin" && u.Status == "active" {
			n, err := s.CountAdmins()
			if err != nil {
				return err
			}
			if n <= 1 {
				return errors.New("cannot demote last admin")
			}
		}
	}
	_, err := s.db.Exec(`UPDATE users SET role=? WHERE id=?`, role, id)
	return err
}

func (s *Store) AdminSetPassword(id int64, password string, mustChange bool) error {
	hash, err := HashPassword(password, 12)
	if err != nil {
		return err
	}
	mustCh := 0
	if mustChange {
		mustCh = 1
	}
	if _, err := s.db.Exec(
		`UPDATE users SET password_hash=?, must_change_password=? WHERE id=?`,
		hash, mustCh, id); err != nil {
		return err
	}
	return s.DeleteSessionsForUser(id)
}

func (s *Store) DeleteUserCascade(id int64) error {
	u, err := s.GetUserByID(id)
	if err != nil {
		return err
	}
	if u.Role == "admin" {
		n, err := s.CountAdmins()
		if err != nil {
			return err
		}
		if n <= 1 {
			return errors.New("cannot delete last admin")
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`DELETE FROM api_keys WHERE user_id=?`,
		`DELETE FROM sessions WHERE user_id=?`,
		`DELETE FROM agents   WHERE user_id=?`,
		`DELETE FROM messages WHERE user_id=?`,
		`DELETE FROM users    WHERE id=?`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
