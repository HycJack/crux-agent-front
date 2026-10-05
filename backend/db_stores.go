package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// MemoryEntry represents a stored memory.
type MemoryEntry struct {
	Key       string `json:"key"`
	UserID    string `json:"user_id"`
	Value     string `json:"value"`
	Category  string `json:"category"`
	UpdatedAt string `json:"updated_at"`
}

// ════════════════════════════════════════════════
// DB initialization & migration
// ════════════════════════════════════════════════

func openAppDB(dataDir string) (*sql.DB, error) {
	dbPath := filepath.Join(dataDir, "chat.db")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrateAppDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func migrateAppDB(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			nickname TEXT DEFAULT '',
			avatar TEXT DEFAULT '',
			role TEXT DEFAULT 'user',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_users_username ON app_users(username);

		CREATE TABLE IF NOT EXISTS app_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			title TEXT DEFAULT '',
			agent_ids TEXT DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_app_sessions_user ON app_sessions(user_id, updated_at DESC);

		CREATE TABLE IF NOT EXISTS app_invite_codes (
			code TEXT PRIMARY KEY,
			created_by TEXT NOT NULL,
			max_uses INTEGER DEFAULT 5,
			use_count INTEGER DEFAULT 0,
			used_by TEXT DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS app_teams (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			coordinator_id TEXT DEFAULT 'default',
			members TEXT DEFAULT '[]',
			builtin INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_teams_user ON app_teams(user_id);

		CREATE TABLE IF NOT EXISTS app_memories (
			key TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			value TEXT DEFAULT '',
			category TEXT DEFAULT 'memory',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (key, user_id)
		);

		CREATE TABLE IF NOT EXISTS app_runtime_skills (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			trigger_text TEXT DEFAULT '',
			steps TEXT DEFAULT '[]',
			tools TEXT DEFAULT '[]',
			env_vars TEXT DEFAULT '{}',
			prompt TEXT DEFAULT '',
			state TEXT DEFAULT 'active',
			version INTEGER DEFAULT 1,
			created_by TEXT DEFAULT 'runtime',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			use_count INTEGER DEFAULT 0,
			patch_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			last_used DATETIME,
			last_patched DATETIME,
			source_session TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_rtskills_user ON app_runtime_skills(user_id);
		CREATE INDEX IF NOT EXISTS idx_rtskills_state ON app_runtime_skills(state);
	`)
	return err
}

// migrateJSONToDB reads JSON files and imports data into SQLite (one-time).
// It looks in both the current dataDir and the legacy .hermes-chat directory.
func migrateJSONToDB(db *sql.DB, dataDir string) {
	// Try legacy directory first (where JSON files were stored before DB migration)
	home, _ := os.UserHomeDir()
	legacyDir := filepath.Join(home, ".hermes-chat")
	if legacyDir == dataDir {
		legacyDir = ""
	}

	dirs := []string{dataDir}
	if legacyDir != "" {
		dirs = append(dirs, legacyDir)
	}

	for _, dir := range dirs {
		migrateUsersJSON(db, dir)
		migrateSessionsJSON(db, dir)
		migrateInviteCodesJSON(db, dir)
		migrateTeamsJSON(db, dir)
		migrateMemoriesJSON(db, dir)
		migrateRuntimeSkillsJSON(db, dir)
	}
}

func migrateUsersJSON(db *sql.DB, dataDir string) {
	path := filepath.Join(dataDir, "users.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // no file, skip
	}
	var items []struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		Password  string `json:"password"`
		Nickname  string `json:"nickname"`
		Avatar    string `json:"avatar"`
		Role      string `json:"role"`
		CreatedAt string `json:"created_at"`
	}
	if json.Unmarshal(data, &items) != nil || len(items) == 0 {
		return
	}
	// Check if DB already has users
	var count int
	db.QueryRow("SELECT COUNT(*) FROM app_users").Scan(&count)
	if count > 0 {
		return
	}
	for _, u := range items {
		db.Exec("INSERT OR IGNORE INTO app_users (id, username, password, nickname, avatar, role, created_at) VALUES (?,?,?,?,?,?,?)",
			u.ID, u.Username, u.Password, u.Nickname, u.Avatar, u.Role, u.CreatedAt)
	}
	// Rename old file
	os.Rename(path, path+".bak")
	log.Printf("[MIGRATE] Imported %d users from JSON", len(items))
}

func migrateSessionsJSON(db *sql.DB, dataDir string) {
	path := filepath.Join(dataDir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var items []struct {
		ID        string   `json:"id"`
		UserID    string   `json:"user_id"`
		Title     string   `json:"title"`
		AgentIDs  []string `json:"agent_ids"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
	}
	if json.Unmarshal(data, &items) != nil || len(items) == 0 {
		return
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM app_sessions").Scan(&count)
	if count > 0 {
		return
	}
	for _, s := range items {
		agentIDs, _ := json.Marshal(s.AgentIDs)
		db.Exec("INSERT OR IGNORE INTO app_sessions (id, user_id, title, agent_ids, created_at, updated_at) VALUES (?,?,?,?,?,?)",
			s.ID, s.UserID, s.Title, string(agentIDs), s.CreatedAt, s.UpdatedAt)
	}
	os.Rename(path, path+".bak")
	log.Printf("[MIGRATE] Imported %d sessions from JSON", len(items))
}

func migrateInviteCodesJSON(db *sql.DB, dataDir string) {
	path := filepath.Join(dataDir, "invite_codes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var items []struct {
		Code      string   `json:"code"`
		CreatedBy string   `json:"created_by"`
		MaxUses   int      `json:"max_uses"`
		UseCount  int      `json:"use_count"`
		UsedBy    []string `json:"used_by"`
		CreatedAt string   `json:"created_at"`
	}
	if json.Unmarshal(data, &items) != nil || len(items) == 0 {
		return
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM app_invite_codes").Scan(&count)
	if count > 0 {
		return
	}
	for _, ic := range items {
		usedBy, _ := json.Marshal(ic.UsedBy)
		db.Exec("INSERT OR IGNORE INTO app_invite_codes (code, created_by, max_uses, use_count, used_by, created_at) VALUES (?,?,?,?,?,?)",
			ic.Code, ic.CreatedBy, ic.MaxUses, ic.UseCount, string(usedBy), ic.CreatedAt)
	}
	os.Rename(path, path+".bak")
	log.Printf("[MIGRATE] Imported %d invite codes from JSON", len(items))
}

func migrateTeamsJSON(db *sql.DB, dataDir string) {
	path := filepath.Join(dataDir, "teams.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var items []Team
	if json.Unmarshal(data, &items) != nil || len(items) == 0 {
		return
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM app_teams").Scan(&count)
	if count > 0 {
		return
	}
	for _, t := range items {
		members, _ := json.Marshal(t.Members)
		builtin := 0
		if t.Builtin {
			builtin = 1
		}
		db.Exec("INSERT OR IGNORE INTO app_teams (id, user_id, name, description, coordinator_id, members, builtin, created_at) VALUES (?,?,?,?,?,?,?,?)",
			t.ID, t.UserID, t.Name, t.Description, t.CoordinatorID, string(members), builtin, t.CreatedAt)
	}
	os.Rename(path, path+".bak")
	log.Printf("[MIGRATE] Imported %d teams from JSON", len(items))
}

func migrateMemoriesJSON(db *sql.DB, dataDir string) {
	path := filepath.Join(dataDir, "memory", "memories.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var items []struct {
		Key       string `json:"key"`
		Value     string `json:"value"`
		Category  string `json:"category"`
		UpdatedAt string `json:"updated_at"`
	}
	if json.Unmarshal(data, &items) != nil || len(items) == 0 {
		return
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM app_memories").Scan(&count)
	if count > 0 {
		return
	}
	for _, m := range items {
		db.Exec("INSERT OR IGNORE INTO app_memories (key, user_id, value, category, updated_at) VALUES (?,?,?,?,?)",
			m.Key, "", m.Value, m.Category, m.UpdatedAt)
	}
	os.Rename(path, path+".bak")
	log.Printf("[MIGRATE] Imported %d memories from JSON", len(items))
}

func migrateRuntimeSkillsJSON(db *sql.DB, dataDir string) {
	path := filepath.Join(dataDir, "runtime_skills.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var items []RuntimeSkill
	if json.Unmarshal(data, &items) != nil || len(items) == 0 {
		return
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM app_runtime_skills").Scan(&count)
	if count > 0 {
		return
	}
	for _, sk := range items {
		steps, _ := json.Marshal(sk.Steps)
		tools, _ := json.Marshal(sk.Tools)
		envVars, _ := json.Marshal(sk.EnvVars)
		db.Exec(`INSERT OR IGNORE INTO app_runtime_skills 
			(id, user_id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			sk.ID, "", sk.Name, sk.Description, sk.Trigger, string(steps), string(tools), string(envVars),
			sk.Prompt, sk.State, sk.Version, sk.CreatedBy, sk.CreatedAt, sk.UpdatedAt,
			sk.UseCount, sk.PatchCount, sk.FailCount, sk.LastUsed, sk.LastPatched, sk.SourceSession)
	}
	os.Rename(path, path+".bak")
	log.Printf("[MIGRATE] Imported %d runtime skills from JSON", len(items))
}

// ════════════════════════════════════════════════
// UserStore (SQLite)
// ════════════════════════════════════════════════

type UserStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// Create inserts a new user. The very first user created becomes the admin
// bootstrap account — callers that provision machine-owned accounts (e.g. the
// messaging gateway) must use CreateInternal so they can never claim that slot.
func (s *UserStore) Create(username, password, nickname string) (*User, error) {
	return s.create(username, password, nickname, true)
}

// CreateInternal provisions a service account for an external system (gateway
// bots). The role is always "user" and the password is never usable for login
// because callers pass a cryptographically random secret.
func (s *UserStore) CreateInternal(username, password, nickname string) (*User, error) {
	return s.create(username, password, nickname, false)
}

func (s *UserStore) create(username, password, nickname string, allowAdminBootstrap bool) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check duplicate
	var exists int
	s.db.QueryRow("SELECT COUNT(*) FROM app_users WHERE username = ?", username).Scan(&exists)
	if exists > 0 {
		return nil, fmt.Errorf("username already exists")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	role := "user"
	// Bootstrap the first *human* account as admin. Keying off "no admin exists
	// yet" rather than "table is empty" means a gateway-provisioned service
	// account (which is never eligible for admin) can no longer consume the
	// admin slot and leave the deployment without any administrator.
	if allowAdminBootstrap {
		var adminCount int
		s.db.QueryRow("SELECT COUNT(*) FROM app_users WHERE role = 'admin'").Scan(&adminCount)
		if adminCount == 0 {
			role = "admin"
		}
	}
	user := &User{
		ID:        fmt.Sprintf("user-%d", time.Now().UnixNano()),
		Username:  username,
		Password:  string(hash),
		Nickname:  nickname,
		Role:      role,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	_, err = s.db.Exec("INSERT INTO app_users (id, username, password, nickname, role, created_at) VALUES (?,?,?,?,?,?)",
		user.ID, user.Username, user.Password, user.Nickname, user.Role, user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserStore) Authenticate(username, password string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user := s.getByUsername(username)
	if user == nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return user, nil
}

func (s *UserStore) GetByID(id string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getByID(id)
}

func (s *UserStore) GetByUsername(username string) *User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getByUsername(username)
}

func (s *UserStore) List() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, username, password, nickname, avatar, role, created_at FROM app_users")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*User
	for rows.Next() {
		u := &User{}
		rows.Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.Avatar, &u.Role, &u.CreatedAt)
		result = append(result, u)
	}
	return result
}

func (s *UserStore) Update(id, nickname, avatar string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sets := []string{}
	args := []any{}
	if nickname != "" {
		sets = append(sets, "nickname = ?")
		args = append(args, nickname)
	}
	if avatar != "" {
		sets = append(sets, "avatar = ?")
		args = append(args, avatar)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := s.db.Exec("UPDATE app_users SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}

func (s *UserStore) getByID(id string) *User {
	u := &User{}
	err := s.db.QueryRow("SELECT id, username, password, nickname, avatar, role, created_at FROM app_users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.Avatar, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil
	}
	return u
}

func (s *UserStore) getByUsername(username string) *User {
	u := &User{}
	err := s.db.QueryRow("SELECT id, username, password, nickname, avatar, role, created_at FROM app_users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.Avatar, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil
	}
	return u
}

// ════════════════════════════════════════════════
// SessionStore (SQLite)
// ════════════════════════════════════════════════

type SessionStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) ListByUser(userID string) []*SessionMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT id, user_id, title, agent_ids, created_at, updated_at FROM app_sessions WHERE user_id = ? ORDER BY updated_at DESC", userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*SessionMeta
	for rows.Next() {
		sess := &SessionMeta{}
		var agentIDs string
		rows.Scan(&sess.ID, &sess.UserID, &sess.Title, &agentIDs, &sess.CreatedAt, &sess.UpdatedAt)
		json.Unmarshal([]byte(agentIDs), &sess.AgentIDs)
		result = append(result, sess)
	}
	return result
}

func (s *SessionStore) Get(id string) *SessionMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess := &SessionMeta{}
	var agentIDs string
	err := s.db.QueryRow("SELECT id, user_id, title, agent_ids, created_at, updated_at FROM app_sessions WHERE id = ?", id).
		Scan(&sess.ID, &sess.UserID, &sess.Title, &agentIDs, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil
	}
	json.Unmarshal([]byte(agentIDs), &sess.AgentIDs)
	return sess
}

func (s *SessionStore) Create(meta *SessionMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if meta.Title == "" {
		meta.Title = "New Chat"
	}
	now := time.Now().Format(time.RFC3339)
	if meta.CreatedAt == "" {
		meta.CreatedAt = now
	}
	if meta.UpdatedAt == "" {
		meta.UpdatedAt = now
	}
	agentIDs, _ := json.Marshal(meta.AgentIDs)
	s.db.Exec("INSERT INTO app_sessions (id, user_id, title, agent_ids, created_at, updated_at) VALUES (?,?,?,?,?,?)",
		meta.ID, meta.UserID, meta.Title, string(agentIDs), meta.CreatedAt, meta.UpdatedAt)
}

func (s *SessionStore) Touch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE app_sessions SET updated_at = ? WHERE id = ?", time.Now().Format(time.RFC3339), id)
}

func (s *SessionStore) UpdateTitle(id, title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("UPDATE app_sessions SET title = ?, updated_at = ? WHERE id = ?", title, time.Now().Format(time.RFC3339), id)
}

func (s *SessionStore) Delete(id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec("DELETE FROM app_sessions WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

// ════════════════════════════════════════════════
// InviteCodeStore (SQLite)
// ════════════════════════════════════════════════

type InviteCodeStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewInviteCodeStore(db *sql.DB) *InviteCodeStore {
	return &InviteCodeStore{db: db}
}

func (s *InviteCodeStore) Create(createdBy string) *InviteCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	ic := &InviteCode{
		Code:      generateInviteCode(),
		CreatedBy: createdBy,
		MaxUses:   5,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.db.Exec("INSERT INTO app_invite_codes (code, created_by, max_uses, use_count, used_by, created_at) VALUES (?,?,?,?,?,?)",
		ic.Code, ic.CreatedBy, ic.MaxUses, 0, "[]", ic.CreatedAt)
	return ic
}

func (s *InviteCodeStore) ValidateAndUse(code, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var useCount, maxUses int
	var usedByStr string
	err := s.db.QueryRow("SELECT use_count, max_uses, used_by FROM app_invite_codes WHERE code = ?", code).
		Scan(&useCount, &maxUses, &usedByStr)
	if err != nil {
		return fmt.Errorf("invalid invite code")
	}
	if useCount >= maxUses {
		return fmt.Errorf("invite code has reached maximum uses")
	}
	var usedBy []string
	json.Unmarshal([]byte(usedByStr), &usedBy)
	if userID != "" {
		usedBy = append(usedBy, userID)
	}
	usedByJSON, _ := json.Marshal(usedBy)
	s.db.Exec("UPDATE app_invite_codes SET use_count = use_count + 1, used_by = ? WHERE code = ?", string(usedByJSON), code)
	return nil
}

func (s *InviteCodeStore) SetUsedBy(code, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var usedByStr string
	s.db.QueryRow("SELECT used_by FROM app_invite_codes WHERE code = ?", code).Scan(&usedByStr)
	var usedBy []string
	json.Unmarshal([]byte(usedByStr), &usedBy)
	usedBy = append(usedBy, userID)
	usedByJSON, _ := json.Marshal(usedBy)
	s.db.Exec("UPDATE app_invite_codes SET used_by = ? WHERE code = ?", string(usedByJSON), code)
}

func (s *InviteCodeStore) ListByUser(userID string) []*InviteCode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT code, created_by, max_uses, use_count, used_by, created_at FROM app_invite_codes WHERE created_by = ?", userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*InviteCode
	for rows.Next() {
		ic := &InviteCode{}
		var usedByStr string
		rows.Scan(&ic.Code, &ic.CreatedBy, &ic.MaxUses, &ic.UseCount, &usedByStr, &ic.CreatedAt)
		json.Unmarshal([]byte(usedByStr), &ic.UsedBy)
		result = append(result, ic)
	}
	return result
}

func (s *InviteCodeStore) GetByUser(userID string) *InviteCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Find existing
	rows, err := s.db.Query("SELECT code, created_by, max_uses, use_count, used_by, created_at FROM app_invite_codes WHERE created_by = ? LIMIT 1", userID)
	if err == nil {
		defer rows.Close()
		if rows.Next() {
			ic := &InviteCode{}
			var usedByStr string
			rows.Scan(&ic.Code, &ic.CreatedBy, &ic.MaxUses, &ic.UseCount, &usedByStr, &ic.CreatedAt)
			json.Unmarshal([]byte(usedByStr), &ic.UsedBy)
			return ic
		}
	}
	// Create new
	ic := &InviteCode{
		Code:      generateInviteCode(),
		CreatedBy: userID,
		MaxUses:   5,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	s.db.Exec("INSERT INTO app_invite_codes (code, created_by, max_uses, use_count, used_by, created_at) VALUES (?,?,?,?,?,?)",
		ic.Code, ic.CreatedBy, ic.MaxUses, 0, "[]", ic.CreatedAt)
	return ic
}

// ════════════════════════════════════════════════
// TeamStore (SQLite)
// ════════════════════════════════════════════════

type TeamStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewTeamStore(db *sql.DB) *TeamStore {
	return &TeamStore{db: db}
}

func (s *TeamStore) List(userID string, isAdmin bool) []*Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rows *sql.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query("SELECT id, user_id, name, description, coordinator_id, members, builtin, created_at FROM app_teams")
	} else {
		rows, err = s.db.Query("SELECT id, user_id, name, description, coordinator_id, members, builtin, created_at FROM app_teams WHERE user_id = ? OR builtin = 1", userID)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*Team
	for rows.Next() {
		t := &Team{}
		var membersStr string
		var builtin int
		rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.CoordinatorID, &membersStr, &builtin, &t.CreatedAt)
		json.Unmarshal([]byte(membersStr), &t.Members)
		t.Builtin = builtin == 1
		result = append(result, t)
	}
	return result
}

func (s *TeamStore) Get(id string) *Team {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := &Team{}
	var membersStr string
	var builtin int
	err := s.db.QueryRow("SELECT id, user_id, name, description, coordinator_id, members, builtin, created_at FROM app_teams WHERE id = ?", id).
		Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.CoordinatorID, &membersStr, &builtin, &t.CreatedAt)
	if err != nil {
		return nil
	}
	json.Unmarshal([]byte(membersStr), &t.Members)
	t.Builtin = builtin == 1
	return t
}

func (s *TeamStore) Create(t *Team) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, _ := json.Marshal(t.Members)
	builtin := 0
	if t.Builtin {
		builtin = 1
	}
	s.db.Exec("INSERT INTO app_teams (id, user_id, name, description, coordinator_id, members, builtin, created_at) VALUES (?,?,?,?,?,?,?,?)",
		t.ID, t.UserID, t.Name, t.Description, t.CoordinatorID, string(members), builtin, t.CreatedAt)
}

func (s *TeamStore) Update(t *Team) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, _ := json.Marshal(t.Members)
	builtin := 0
	if t.Builtin {
		builtin = 1
	}
	s.db.Exec("UPDATE app_teams SET name=?, description=?, coordinator_id=?, members=?, builtin=? WHERE id=?",
		t.Name, t.Description, t.CoordinatorID, string(members), builtin, t.ID)
}

func (s *TeamStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM app_teams WHERE id = ?", id)
}

// ════════════════════════════════════════════════
// MemoryStore (SQLite)
// ════════════════════════════════════════════════

type MemoryStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewMemoryStore(db *sql.DB) *MemoryStore {
	return &MemoryStore{db: db}
}

func (s *MemoryStore) Save(key, userID, value, category string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("INSERT OR REPLACE INTO app_memories (key, user_id, value, category, updated_at) VALUES (?,?,?,?,?)",
		key, userID, value, category, time.Now().Format(time.RFC3339))
}

func (s *MemoryStore) Get(key, userID string) (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var value, category string
	err := s.db.QueryRow("SELECT value, category FROM app_memories WHERE key = ? AND user_id = ?", key, userID).Scan(&value, &category)
	if err != nil {
		return "", ""
	}
	return value, category
}

func (s *MemoryStore) List(userID string) []MemoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query("SELECT key, user_id, value, category, updated_at FROM app_memories WHERE user_id = ? ORDER BY updated_at DESC", userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []MemoryEntry
	for rows.Next() {
		m := MemoryEntry{}
		rows.Scan(&m.Key, &m.UserID, &m.Value, &m.Category, &m.UpdatedAt)
		result = append(result, m)
	}
	return result
}

func (s *MemoryStore) Delete(key, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec("DELETE FROM app_memories WHERE key = ? AND user_id = ?", key, userID)
}

func (s *MemoryStore) Search(query, userID string) []MemoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := "%" + query + "%"
	rows, err := s.db.Query("SELECT key, user_id, value, category, updated_at FROM app_memories WHERE user_id = ? AND (key LIKE ? OR value LIKE ?) ORDER BY updated_at DESC LIMIT 20",
		userID, q, q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []MemoryEntry
	for rows.Next() {
		m := MemoryEntry{}
		rows.Scan(&m.Key, &m.UserID, &m.Value, &m.Category, &m.UpdatedAt)
		result = append(result, m)
	}
	return result
}

// ════════════════════════════════════════════════
// RuntimeSkillStore (SQLite)
// ════════════════════════════════════════════════

type RuntimeSkillStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewRuntimeSkillStore(db *sql.DB) *RuntimeSkillStore {
	return &RuntimeSkillStore{db: db}
}

func (s *RuntimeSkillStore) Get(id string) *RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getByID(id)
}

// GetForUser returns the skill only if it belongs to userID. Use this in any
// request handler — Get leaks other tenants' skills (which embed prompts,
// steps and env var names).
func (s *RuntimeSkillStore) GetForUser(id, userID string) *RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk := s.getByID(id)
	if sk == nil || sk.UserID != userID {
		return nil
	}
	return sk
}

func (s *RuntimeSkillStore) List() []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills ORDER BY updated_at DESC", nil)
}

func (s *RuntimeSkillStore) ListByUser(userID string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE user_id = ? ORDER BY updated_at DESC", userID)
}

func (s *RuntimeSkillStore) Create(sk *RuntimeSkill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sk.ID == "" {
		sk.ID = fmt.Sprintf("rtskill-%d", time.Now().UnixNano())
	}
	now := time.Now()
	sk.CreatedAt = now
	sk.UpdatedAt = now
	if sk.Version == 0 {
		sk.Version = 1
	}
	if sk.State == "" {
		sk.State = "active"
	}
	steps, _ := json.Marshal(sk.Steps)
	tools, _ := json.Marshal(sk.Tools)
	envVars, _ := json.Marshal(sk.EnvVars)
	_, err := s.db.Exec(`INSERT INTO app_runtime_skills 
		(id, user_id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sk.ID, sk.UserID, sk.Name, sk.Description, sk.Trigger, string(steps), string(tools), string(envVars),
		sk.Prompt, sk.State, sk.Version, sk.CreatedBy, sk.CreatedAt, sk.UpdatedAt,
		sk.UseCount, sk.PatchCount, sk.FailCount, sk.LastUsed, sk.LastPatched, sk.SourceSession)
	return err
}

func (s *RuntimeSkillStore) Update(sk *RuntimeSkill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.getByID(sk.ID)
	if existing == nil {
		return fmt.Errorf("not found")
	}
	sk.CreatedAt = existing.CreatedAt
	sk.CreatedBy = existing.CreatedBy
	sk.SourceSession = existing.SourceSession
	sk.UpdatedAt = time.Now()
	steps, _ := json.Marshal(sk.Steps)
	tools, _ := json.Marshal(sk.Tools)
	envVars, _ := json.Marshal(sk.EnvVars)
	_, err := s.db.Exec(`UPDATE app_runtime_skills SET name=?, description=?, trigger_text=?, steps=?, tools=?, env_vars=?, prompt=?, state=?, version=?, updated_at=?, use_count=?, patch_count=?, fail_count=?, last_used=?, last_patched=? WHERE id=?`,
		sk.Name, sk.Description, sk.Trigger, string(steps), string(tools), string(envVars),
		sk.Prompt, sk.State, sk.Version, sk.UpdatedAt, sk.UseCount, sk.PatchCount, sk.FailCount, sk.LastUsed, sk.LastPatched, sk.ID)
	return err
}

func (s *RuntimeSkillStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec("DELETE FROM app_runtime_skills WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

func (s *RuntimeSkillStore) ListByState(state string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE state = ?", state)
}

func (s *RuntimeSkillStore) ListByTool(toolName string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE tools LIKE ?", "%"+toolName+"%")
}

// ── User-scoped variants, for HTTP request handlers ──────────────────────
// The unscoped methods above are still used by the engine's internal
// activation path; anything reachable from a route must use these.

func (s *RuntimeSkillStore) ListByStateForUser(state, userID string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE state = ? AND user_id = ?", state, userID)
}

func (s *RuntimeSkillStore) ListByToolForUser(toolName, userID string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE tools LIKE ? AND user_id = ?", "%"+toolName+"%", userID)
}

func (s *RuntimeSkillStore) SearchForUser(query, userID string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := "%" + strings.ToLower(query) + "%"
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE (LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(trigger_text) LIKE ?) AND user_id = ?", q, q, q, userID)
}

func (s *RuntimeSkillStore) Search(query string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := "%" + strings.ToLower(query) + "%"
	return s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(trigger_text) LIKE ?", q, q, q)
}

func (s *RuntimeSkillStore) getByID(id string) *RuntimeSkill {
	list := s.query("SELECT id, name, description, trigger_text, steps, tools, env_vars, prompt, state, version, created_by, created_at, updated_at, use_count, patch_count, fail_count, last_used, last_patched, source_session, user_id FROM app_runtime_skills WHERE id = ?", id)
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

func (s *RuntimeSkillStore) query(sqlStr string, args ...any) []*RuntimeSkill {
	rows, err := s.db.Query(sqlStr, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []*RuntimeSkill
	for rows.Next() {
		sk := &RuntimeSkill{}
		var stepsStr, toolsStr, envVarsStr string
		var lastUsed, lastPatched sql.NullTime
		// A scan error previously left a zero-valued struct in the result slice,
		// which then failed downstream ownership checks. Skip and log instead.
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Description, &sk.Trigger, &stepsStr, &toolsStr, &envVarsStr,
			&sk.Prompt, &sk.State, &sk.Version, &sk.CreatedBy, &sk.CreatedAt, &sk.UpdatedAt,
			&sk.UseCount, &sk.PatchCount, &sk.FailCount, &lastUsed, &lastPatched, &sk.SourceSession, &sk.UserID); err != nil {
			log.Printf("runtime_skill: scan error: %v", err)
			continue
		}
		json.Unmarshal([]byte(stepsStr), &sk.Steps)
		json.Unmarshal([]byte(toolsStr), &sk.Tools)
		json.Unmarshal([]byte(envVarsStr), &sk.EnvVars)
		if lastUsed.Valid {
			sk.LastUsed = lastUsed.Time
		}
		if lastPatched.Valid {
			sk.LastPatched = lastPatched.Time
		}
		result = append(result, sk)
	}
	return result
}
