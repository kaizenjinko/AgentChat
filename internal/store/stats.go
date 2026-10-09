package store

import "database/sql"

type Stats struct {
	Rooms    int64 `json:"rooms"`
	Messages int64 `json:"messages"`
	Agents   int64 `json:"agents"`
	Keys     int64 `json:"keys"`
}

type SystemStats struct {
	Users      int64 `json:"users"`
	Rooms      int64 `json:"rooms"`
	Messages   int64 `json:"messages"`
	ActiveKeys int64 `json:"active_keys"`
}

type AgentStat struct {
	Name     string `json:"name"`
	Messages int64  `json:"messages"`
	LastAt   string `json:"last_at"`
}

type AgentStatWithOwner struct {
	AgentStat
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type RoomWithOwner struct {
	Room     string `json:"room"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Count    int64  `json:"count"`
	LastID   int64  `json:"last_id"`
	LastAt   string `json:"last_at"`
}

func (s *Store) UserStats(userID int64) (*Stats, error) {
	var st Stats
	err := s.db.QueryRow(
		`SELECT
		   (SELECT COUNT(*) FROM rooms WHERE user_id=?) AS rooms,
		   (SELECT COUNT(*) FROM messages WHERE user_id=?) AS messages,
		   (SELECT COUNT(*) FROM agents WHERE user_id=?) AS agents,
		   (SELECT COUNT(*) FROM api_keys WHERE user_id=? AND status='active') AS keys`,
		userID, userID, userID, userID,
	).Scan(&st.Rooms, &st.Messages, &st.Agents, &st.Keys)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) SystemStats() (*SystemStats, error) {
	var st SystemStats
	err := s.db.QueryRow(
		`SELECT
		   (SELECT COUNT(*) FROM users) AS users,
		   (SELECT COUNT(*) FROM rooms) AS rooms,
		   (SELECT COUNT(*) FROM messages) AS messages,
		   (SELECT COUNT(*) FROM api_keys WHERE status='active') AS active_keys`,
	).Scan(&st.Users, &st.Rooms, &st.Messages, &st.ActiveKeys)
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) ListAgents(userID int64) ([]AgentStat, error) {
	rows, err := s.db.Query(
		`SELECT agent AS name, COUNT(*) AS messages, MAX(created_at) AS last_at
		 FROM messages WHERE user_id=? GROUP BY agent ORDER BY messages DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]AgentStat, 0)
	for rows.Next() {
		var (
			a      AgentStat
			lastAt sql.NullString
		)
		if err := rows.Scan(&a.Name, &a.Messages, &lastAt); err != nil {
			return nil, err
		}
		a.LastAt = lastAt.String
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) ListAgentsAll() ([]AgentStatWithOwner, error) {
	rows, err := s.db.Query(
		`SELECT m.agent AS name, COUNT(*) AS messages, MAX(m.created_at) AS last_at,
		        m.user_id AS user_id, u.username AS username
		 FROM messages m JOIN users u ON u.id = m.user_id
		 GROUP BY m.user_id, m.agent
		 ORDER BY messages DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]AgentStatWithOwner, 0)
	for rows.Next() {
		var (
			a      AgentStatWithOwner
			lastAt sql.NullString
		)
		if err := rows.Scan(&a.Name, &a.Messages, &lastAt, &a.UserID, &a.Username); err != nil {
			return nil, err
		}
		a.LastAt = lastAt.String
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *Store) ListRoomsAll() ([]RoomWithOwner, error) {
	rows, err := s.db.Query(
		`SELECT r.name, r.user_id, u.username,
		        COALESCE(m.count, 0), COALESCE(m.last_id, 0), m.last_at
		 FROM rooms r JOIN users u ON u.id = r.user_id
		 LEFT JOIN (
		   SELECT room, user_id, COUNT(*) AS count, MAX(id) AS last_id, MAX(created_at) AS last_at
		   FROM messages GROUP BY user_id, room
		 ) m ON m.room = r.name AND m.user_id = r.user_id
		 ORDER BY COALESCE(m.last_id, 0) DESC, r.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rooms := make([]RoomWithOwner, 0)
	for rows.Next() {
		var (
			r      RoomWithOwner
			lastAt sql.NullString
		)
		if err := rows.Scan(&r.Room, &r.UserID, &r.Username, &r.Count, &r.LastID, &lastAt); err != nil {
			return nil, err
		}
		r.LastAt = lastAt.String
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}
