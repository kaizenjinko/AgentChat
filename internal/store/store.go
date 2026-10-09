package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"agentchat/internal/filter"

	_ "modernc.org/sqlite"
)

// Message is the wire + storage representation of a chat message.
type Message struct {
	ID        int64    `json:"id"`
	Agent     string   `json:"agent"`
	Room      string   `json:"room"`
	UserID    int64    `json:"user_id"`
	Content   string   `json:"content"`
	Mentions  []string `json:"mentions"`
	ReplyTo   *int64   `json:"reply_to"`
	CreatedAt string   `json:"created_at"`
}

// Room aggregates per-room statistics.
type Room struct {
	Room   string `json:"room"`
	Count  int64  `json:"count"`
	LastID int64  `json:"last_id"`
	LastAt string `json:"last_at"`
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (and initializes) the SQLite database at path.
func Open(path string) (*Store, error) {
	// Apply pragmas through the DSN so every pooled connection inherits them.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=wal_autocheckpoint(1000)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc/sqlite writes serialize anyway; a single connection keeps the
	// per-connection pragmas sticky and avoids SQLITE_BUSY between goroutines.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init() error {
	stmts := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA wal_autocheckpoint=1000",
		"PRAGMA busy_timeout=5000",
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent TEXT NOT NULL,
			room TEXT NOT NULL DEFAULT 'general',
			content TEXT NOT NULL,
			mentions TEXT DEFAULT '[]',
			reply_to INTEGER,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		"CREATE INDEX IF NOT EXISTS idx_room_id ON messages(room, id)",
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("init %q: %w", q, err)
		}
	}
	return s.Migrate()
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// Insert stores a message and returns it with its assigned id/created_at.
func (s *Store) Insert(userID int64, agent, room, content string, mentions []string, replyTo *int64) (*Message, error) {
	mj := encodeMentions(mentions)
	res, err := s.db.Exec(
		`INSERT INTO messages (user_id, agent, room, content, mentions, reply_to) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, agent, room, content, mj, replyTo,
	)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO rooms (user_id, name) VALUES (?, ?)`, userID, room); err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	var created sql.NullString
	if err := s.db.QueryRow(`SELECT created_at FROM messages WHERE id = ?`, id).Scan(&created); err != nil {
		return nil, err
	}
	m := &Message{
		ID:        id,
		UserID:    userID,
		Agent:     agent,
		Room:      room,
		Content:   content,
		Mentions:  mentions,
		ReplyTo:   replyTo,
		CreatedAt: created.String,
	}
	if m.Mentions == nil {
		m.Mentions = []string{}
	}
	return m, nil
}

// encodeMentions serialises a mention slice to the stored flat JSON array form.
func encodeMentions(mentions []string) string {
	if len(mentions) == 0 {
		return "[]"
	}
	n := 2
	for _, s := range mentions {
		n += len(s) + 3
	}
	b := make([]byte, 0, n)
	b = append(b, '[')
	for i, s := range mentions {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendJSONString(b, s)
	}
	b = append(b, ']')
	return string(b)
}

// appendJSONString appends a JSON-quoted string. Mention names are restricted to
// [A-Za-z0-9_@-] by the parser, so only quote and backslash can need escaping;
// other bytes are copied verbatim.
func appendJSONString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			if c < 0x20 {
				const hex = "0123456789abcdef"
				b = append(b, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
			} else {
				b = append(b, c)
			}
		}
	}
	return append(b, '"')
}

// GetByID returns a single message by id. ids are globally unique, so it is not
// scoped by user; callers must enforce ownership when needed.
func (s *Store) GetByID(id int64) (*Message, error) {
	row := s.db.QueryRow(
		`SELECT id, user_id, agent, room, content, mentions, reply_to, created_at FROM messages WHERE id = ?`, id)
	return scanMessage(row)
}

// MessageBelongsToUser reports whether a message with the given id exists and
// belongs to userID.
func (s *Store) MessageBelongsToUser(userID, id int64) bool {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM messages WHERE id = ? AND user_id = ?`, id, userID).Scan(&n)
	return err == nil
}

// ListRooms returns per-room aggregates ordered by last_id DESC, including
// registered rooms that have no messages yet (count 0, last_id 0).
func (s *Store) ListRooms(userID int64) ([]Room, error) {
	rows, err := s.db.Query(
		`SELECT r.name AS room,
		        COALESCE(m.count, 0) AS count,
		        COALESCE(m.last_id, 0) AS last_id,
		        m.last_at AS last_at
		 FROM rooms r
		 LEFT JOIN (
		   SELECT room, COUNT(*) AS count, MAX(id) AS last_id, MAX(created_at) AS last_at
		   FROM messages WHERE user_id = ? GROUP BY room
		 ) m ON m.room = r.name
		 WHERE r.user_id = ?
		 ORDER BY COALESCE(m.last_id, 0) DESC, r.name ASC`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rooms := make([]Room, 0)
	for rows.Next() {
		var r Room
		var lastAt sql.NullString
		if err := rows.Scan(&r.Room, &r.Count, &r.LastID, &lastAt); err != nil {
			return nil, err
		}
		r.LastAt = lastAt.String
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}

// CreateRoom registers an empty room for userID. It reports whether a new room
// was created (false when it already existed).
func (s *Store) CreateRoom(userID int64, room string) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO rooms (user_id, name) VALUES (?, ?)`, userID, room)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DeleteRoom deletes all messages in a room owned by userID and unregisters it.
func (s *Store) DeleteRoom(userID int64, room string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM messages WHERE room = ? AND user_id = ?`, room, userID)
	if err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`DELETE FROM rooms WHERE name = ? AND user_id = ?`, room, userID); err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryResult carries the rows plus the resolved paging metadata.
type QueryResult struct {
	Messages []Message
	Limit    int
	Offset   int
	Order    string
}

// Query builds and runs the flexible message query (rooms, mentions, agents,
// id range, content search, ordering and pagination).
func (s *Store) Query(userID int64, params url.Values) (*QueryResult, error) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 8)

	where = append(where, "user_id = ?")
	args = append(args, userID)

	rooms := filter.CSVFirst(params, "room", "rooms")
	switch {
	case len(rooms) == 1:
		where = append(where, "room = ?")
		args = append(args, rooms[0])
	case len(rooms) > 1:
		where = append(where, "room IN ("+placeholders(len(rooms))+")")
		for _, r := range rooms {
			args = append(args, r)
		}
	}

	if v, ok := filter.First(params, "since"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n != 0 {
			where = append(where, "id > ?")
			args = append(args, int64(n))
		}
	}
	if v, ok := filter.First(params, "until"); ok {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n != 0 {
			where = append(where, "id <= ?")
			args = append(args, int64(n))
		}
	}

	agents := filter.CSVFirst(params, "agent", "agents")
	if len(agents) > 0 {
		where = append(where, "agent IN ("+placeholders(len(agents))+")")
		for _, a := range agents {
			args = append(args, a)
		}
	}

	mentions := filter.CSVFirst(params, "mention", "mentions")
	if len(mentions) > 0 {
		all := false
		if m, ok := filter.First(params, "mode"); ok {
			all = strings.ToLower(m) == "all"
		}
		join := " OR "
		if all {
			join = " AND "
		}
		parts := make([]string, len(mentions))
		for i := range mentions {
			parts[i] = "EXISTS (SELECT 1 FROM json_each(messages.mentions) WHERE json_each.value = ?)"
		}
		where = append(where, "("+strings.Join(parts, join)+")")
		for _, m := range mentions {
			args = append(args, m)
		}
	}

	if v, ok := filter.First(params, "has_mentions"); ok {
		want := v != "false" && v != "0"
		if want {
			where = append(where, "(mentions IS NOT NULL AND mentions != '[]')")
		} else {
			where = append(where, "(mentions IS NULL OR mentions = '[]')")
		}
	}

	if v, ok := filter.First(params, "reply_to"); ok && v != "" {
		if v == "null" {
			where = append(where, "reply_to IS NULL")
		} else if n, err := strconv.ParseFloat(v, 64); err == nil {
			where = append(where, "reply_to = ?")
			args = append(args, int64(n))
		}
	}

	if q := params.Get("q"); q != "" {
		where = append(where, "content LIKE ?")
		args = append(args, "%"+q+"%")
	}

	order := "ASC"
	if strings.ToLower(params.Get("order")) == "desc" {
		order = "DESC"
	}

	limit := 200
	if v, ok := filter.First(params, "limit"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := 0
	if v, ok := filter.First(params, "offset"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	if offset < 0 {
		offset = 0
	}

	sqlStr := "SELECT id, user_id, agent, room, content, mentions, reply_to, created_at FROM messages"
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += " ORDER BY id " + order + " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := make([]Message, 0, queryCap(limit))
	for rows.Next() {
		msgs = append(msgs, Message{})
		if err := scanMessageInto(rows, &msgs[len(msgs)-1]); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &QueryResult{Messages: msgs, Limit: limit, Offset: offset, Order: order}, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// queryCap returns a slice capacity hint for a query result: enough to avoid
// repeated growth for typical pages without over-allocating when a LIMIT is
// large but few rows match.
func queryCap(limit int) int {
	if limit < 8 {
		return limit
	}
	if limit > 64 {
		return 64
	}
	return limit
}

// scanner abstracts *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(sc scanner) (*Message, error) {
	m := &Message{}
	if err := scanMessageInto(sc, m); err != nil {
		return nil, err
	}
	return m, nil
}

// scanMessageInto fills dst from a row without allocating a new Message, so
// callers appending into a slice avoid one box per row.
func scanMessageInto(sc scanner, dst *Message) error {
	var (
		mentRaw string
		replyTo sql.NullInt64
		created sql.NullString
	)
	if err := sc.Scan(&dst.ID, &dst.UserID, &dst.Agent, &dst.Room, &dst.Content, &mentRaw, &replyTo, &created); err != nil {
		return err
	}
	dst.Mentions = parseMentionsJSON(mentRaw)
	if replyTo.Valid {
		v := replyTo.Int64
		dst.ReplyTo = &v
	}
	dst.CreatedAt = created.String
	return nil
}

// parseMentionsJSON decodes the stored mentions array (always a flat JSON array
// of strings, e.g. `["coder","reviewer"]` or `[]`) without the reflection cost
// of encoding/json. Falls back to empty on any malformed input.
func parseMentionsJSON(raw string) []string {
	if len(raw) < 2 || raw[0] != '[' {
		return []string{}
	}
	if raw[1] == ']' {
		return []string{}
	}
	out := make([]string, 0, 4)
	i := 1
	for i < len(raw) {
		c := raw[i]
		switch c {
		case ' ', ',', '\t', '\n', '\r':
			i++
		case ']':
			return out
		case '"':
			i++
			start := i
			esc := false
			for i < len(raw) {
				b := raw[i]
				if b == '\\' {
					esc = true
					i += 2
					continue
				}
				if b == '"' {
					break
				}
				i++
			}
			if i > len(raw) {
				return out
			}
			seg := raw[start:i]
			if esc {
				unescaped, ok := unescapeJSONString(seg)
				if !ok {
					return out
				}
				seg = unescaped
			}
			out = append(out, seg)
			i++
		default:
			return out
		}
	}
	return out
}

// unescapeJSONString handles the escape sequences that mentions can contain
// (backslash, quote, and the common control escapes). Returns ok=false for
// sequences it does not decode, signalling the caller to bail out safely.
func unescapeJSONString(s string) (string, bool) {
	hasEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			hasEsc = true
			break
		}
	}
	if !hasEsc {
		return s, true
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b = append(b, s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", false
		}
		switch s[i] {
		case '"', '\\', '/':
			b = append(b, s[i])
		case 'n':
			b = append(b, '\n')
		case 't':
			b = append(b, '\t')
		case 'r':
			b = append(b, '\r')
		case 'b':
			b = append(b, '\b')
		case 'f':
			b = append(b, '\f')
		default:
			return "", false
		}
	}
	return string(b), true
}
