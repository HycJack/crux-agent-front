package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store is the persistence interface.
type Store interface {
	CreateSession(sess *SessionMeta) error
	GetSession(id string) (*SessionMeta, error)
	ListSessions(limit int) ([]SessionMeta, error)
	UpdateSession(id string, patch map[string]any) error
	DeleteSession(id string) error
	AppendMessage(sessionID string, msg *StoredMessage) error
	GetMessages(sessionID string, opts MessageOpts) ([]StoredMessage, error)
	GetMessage(id string) (*StoredMessage, error)
	CountMessages(sessionID string) (int, error)
	MarkCompacted(sessionID string, beforeID string) (int, error)
	SearchMessages(query string, opts SearchOpts) ([]SearchHit, error)
	Set(namespace, key string, value []byte) error
	Get(namespace, key string) ([]byte, error)
	Delete(namespace, key string) error
	List(namespace string) ([]KVPair, error)
	Close() error
}

type SessionMeta struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	ParentID     string    `json:"parent_id,omitempty"`
	TokenCount   int       `json:"token_count"`
	MessageCount int       `json:"message_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type StoredMessage struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	ParentMsgID string          `json:"parent_msg_id,omitempty"`
	Role        string          `json:"role"`
	Content     string          `json:"content"`
	ToolCalls   json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID  string          `json:"tool_call_id,omitempty"`
	Name        string          `json:"name,omitempty"`
	TokenEst    int             `json:"token_est"`
	Compacted   bool            `json:"compacted"`
	CreatedAt   time.Time       `json:"created_at"`
}

type MessageOpts struct {
	Limit            int
	Offset           int
	ExcludeCompacted bool
	Roles            []string
}

type SearchOpts struct {
	Limit     int
	SessionID string
	Roles     []string
}

type SearchHit struct {
	SessionID string
	MessageID string
	Role      string
	Content   string
	Snippet   string
	Rank      float64
}

type KVPair struct {
	Key       string
	Value     []byte
	UpdatedAt time.Time
}

type SQLite struct {
	db     *sql.DB
	hasFTS bool
	logger *slog.Logger
}

func NewSQLite(dsn string) (*SQLite, error) {
	dsn = expandPath(dsn)
	if err := os.MkdirAll(filepath.Dir(dsn), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open("sqlite3", dsn+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &SQLite{db: db, logger: slog.Default()}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	s.initFTS()
	return s, nil
}

func (s *SQLite) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY, title TEXT DEFAULT '', parent_id TEXT DEFAULT '',
			token_count INTEGER DEFAULT 0, message_count INTEGER DEFAULT 0,
			created_at DATETIME, updated_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at DESC);
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL, parent_msg_id TEXT DEFAULT '',
			role TEXT NOT NULL, content TEXT DEFAULT '', tool_calls TEXT DEFAULT '[]',
			tool_call_id TEXT DEFAULT '', name TEXT DEFAULT '',
			token_est INTEGER DEFAULT 0, compacted INTEGER DEFAULT 0, created_at DATETIME,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);
		CREATE TABLE IF NOT EXISTS kv (
			namespace TEXT, key TEXT, value BLOB, updated_at DATETIME,
			PRIMARY KEY (namespace, key)
		);
	`)
	return err
}

func (s *SQLite) initFTS() {
	_, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(content, content=messages, content_rowid=rowid)`)
	if err != nil {
		s.hasFTS = false
		s.logger.Warn("FTS5 not available, falling back to LIKE search", "error", err)
		return
	}
	s.hasFTS = true

	// Create triggers for FTS sync
	s.db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
	END`)
	s.db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, content) VALUES('delete', old.rowid, old.content);
	END`)
	s.db.Exec(`CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
		INSERT INTO messages_fts(messages_fts, rowid, content) VALUES('delete', old.rowid, old.content);
		INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
	END`)
}

// --- Session CRUD ---

func (s *SQLite) CreateSession(sess *SessionMeta) error {
	now := time.Now()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = now
	}
	_, err := s.db.Exec(
		"INSERT INTO sessions (id, title, parent_id, token_count, message_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		sess.ID, sess.Title, sess.ParentID, sess.TokenCount, sess.MessageCount, sess.CreatedAt, sess.UpdatedAt,
	)
	return err
}

func (s *SQLite) GetSession(id string) (*SessionMeta, error) {
	row := s.db.QueryRow("SELECT id, title, parent_id, token_count, message_count, created_at, updated_at FROM sessions WHERE id = ?", id)
	var sess SessionMeta
	err := row.Scan(&sess.ID, &sess.Title, &sess.ParentID, &sess.TokenCount, &sess.MessageCount, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *SQLite) ListSessions(limit int) ([]SessionMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query("SELECT id, title, parent_id, token_count, message_count, created_at, updated_at FROM sessions ORDER BY updated_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionMeta
	for rows.Next() {
		var sess SessionMeta
		if err := rows.Scan(&sess.ID, &sess.Title, &sess.ParentID, &sess.TokenCount, &sess.MessageCount, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *SQLite) UpdateSession(id string, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	var sets []string
	var args []any
	for k, v := range patch {
		sets = append(sets, k+" = ?")
		args = append(args, v)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, id)
	_, err := s.db.Exec("UPDATE sessions SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

func (s *SQLite) DeleteSession(id string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

// --- Message CRUD ---

func (s *SQLite) AppendMessage(sessionID string, msg *StoredMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.SessionID == "" {
		msg.SessionID = sessionID
	}
	toolCallsJSON := msg.ToolCalls
	if toolCallsJSON == nil {
		toolCallsJSON = json.RawMessage("[]")
	}

	_, err := s.db.Exec(
		`INSERT INTO messages (id, session_id, parent_msg_id, role, content, tool_calls, tool_call_id, name, token_est, compacted, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, sessionID, msg.ParentMsgID, msg.Role, msg.Content, toolCallsJSON,
		msg.ToolCallID, msg.Name, msg.TokenEst, msg.Compacted, msg.CreatedAt,
	)
	if err != nil {
		return err
	}

	// Update session counters
	_, err = s.db.Exec(
		"UPDATE sessions SET message_count = message_count + 1, token_count = token_count + ?, updated_at = ? WHERE id = ?",
		msg.TokenEst, time.Now(), sessionID,
	)
	return err
}

func (s *SQLite) GetMessages(sessionID string, opts MessageOpts) ([]StoredMessage, error) {
	query := "SELECT id, session_id, parent_msg_id, role, content, tool_calls, tool_call_id, name, token_est, compacted, created_at FROM messages WHERE session_id = ?"
	args := []any{sessionID}

	if opts.ExcludeCompacted {
		query += " AND compacted = 0"
	}
	if len(opts.Roles) > 0 {
		placeholders := make([]string, len(opts.Roles))
		for i, r := range opts.Roles {
			placeholders[i] = "?"
			args = append(args, r)
		}
		query += " AND role IN (" + strings.Join(placeholders, ",") + ")"
	}

	query += " ORDER BY created_at ASC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []StoredMessage
	for rows.Next() {
		var m StoredMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.ParentMsgID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.Name, &m.TokenEst, &m.Compacted, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *SQLite) GetMessage(id string) (*StoredMessage, error) {
	row := s.db.QueryRow(
		"SELECT id, session_id, parent_msg_id, role, content, tool_calls, tool_call_id, name, token_est, compacted, created_at FROM messages WHERE id = ?", id,
	)
	var m StoredMessage
	err := row.Scan(&m.ID, &m.SessionID, &m.ParentMsgID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.Name, &m.TokenEst, &m.Compacted, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *SQLite) CountMessages(sessionID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sessionID).Scan(&count)
	return count, err
}

// MarkCompacted marks all messages before the given message ID as compacted.
func (s *SQLite) MarkCompacted(sessionID string, beforeID string) (int, error) {
	result, err := s.db.Exec(
		"UPDATE messages SET compacted = 1 WHERE session_id = ? AND id != ? AND compacted = 0 AND created_at < (SELECT created_at FROM messages WHERE id = ?)",
		sessionID, beforeID, beforeID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// --- Search ---

func (s *SQLite) SearchMessages(query string, opts SearchOpts) ([]SearchHit, error) {
	if s.hasFTS {
		return s.searchFTS(query, opts)
	}
	return s.searchLIKE(query, opts)
}

func (s *SQLite) searchFTS(query string, opts SearchOpts) ([]SearchHit, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	sqlQuery := `SELECT m.session_id, m.id, m.role, m.content, snippet(messages_fts, 0, '>>>', '<<<', '...', 32), rank
		FROM messages_fts fts JOIN messages m ON fts.rowid = m.rowid
		WHERE messages_fts MATCH ?`
	args := []any{query}

	if opts.SessionID != "" {
		sqlQuery += " AND m.session_id = ?"
		args = append(args, opts.SessionID)
	}
	sqlQuery += " ORDER BY rank LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.SessionID, &h.MessageID, &h.Role, &h.Content, &h.Snippet, &h.Rank); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, nil
}

func (s *SQLite) searchLIKE(query string, opts SearchOpts) ([]SearchHit, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	sqlQuery := "SELECT session_id, id, role, content FROM messages WHERE content LIKE ?"
	args := []any{"%" + query + "%"}

	if opts.SessionID != "" {
		sqlQuery += " AND session_id = ?"
		args = append(args, opts.SessionID)
	}
	sqlQuery += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.SessionID, &h.MessageID, &h.Role, &h.Content); err != nil {
			return nil, err
		}
		// Generate snippet
		content := h.Content
		if len(content) > 128 {
			content = content[:128] + "..."
		}
		h.Snippet = content
		hits = append(hits, h)
	}
	return hits, nil
}

// --- KV Store ---

func (s *SQLite) Set(namespace, key string, value []byte) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO kv (namespace, key, value, updated_at) VALUES (?, ?, ?, ?)",
		namespace, key, value, time.Now(),
	)
	return err
}

func (s *SQLite) Get(namespace, key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRow("SELECT value FROM kv WHERE namespace = ? AND key = ?", namespace, key).Scan(&value)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (s *SQLite) Delete(namespace, key string) error {
	_, err := s.db.Exec("DELETE FROM kv WHERE namespace = ? AND key = ?", namespace, key)
	return err
}

func (s *SQLite) List(namespace string) ([]KVPair, error) {
	rows, err := s.db.Query("SELECT key, value, updated_at FROM kv WHERE namespace = ? ORDER BY key", namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []KVPair
	for rows.Next() {
		var p KVPair
		if err := rows.Scan(&p.Key, &p.Value, &p.UpdatedAt); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
