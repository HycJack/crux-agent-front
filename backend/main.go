package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"


	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/primitives"
	"github.com/hermes-go/core/store"
	"github.com/hermes-go/core/types"
	"github.com/hermes-go/skills/memory"
	"github.com/hermes-go/skills/terminal"
)

var (
	listenAddr = envOr("LISTEN_ADDR", ":8080")
	uploadDir  = envOr("UPLOAD_DIR", "./uploads")
	jwtSecret  = []byte(envOr("JWT_SECRET", "crux-chat-secret-key-change-me"))
	envMu      sync.Mutex // protects os.Setenv/Unsetenv from concurrent requests
)

func init() {
	// Load .env file from data directory if it exists
	home, _ := os.UserHomeDir()
	envPath := filepath.Join(home, ".crux-chat", ".env")
	if data, err := os.ReadFile(envPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if idx := strings.Index(line, "="); idx > 0 {
				key := strings.TrimSpace(line[:idx])
				val := strings.TrimSpace(line[idx+1:])
				if os.Getenv(key) == "" {
					os.Setenv(key, val)
				}
			}
		}
	}
	jwtSecret = []byte(envOr("JWT_SECRET", ""))
	if len(jwtSecret) == 0 {
		log.Fatalf("FATAL: JWT_SECRET environment variable is required. Generate one with: openssl rand -hex 32")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ════════════════════════════════════════════════
// User

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar,omitempty"`
	Role      string `json:"role"` // admin, user
	CreatedAt string `json:"created_at"`
}





// ════════════════════════════════════════════════
// JWT

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func generateToken(user *User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func parseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// ════════════════════════════════════════════════
// Agent config

type AgentConfig struct {
	ID             string   `json:"id"`
	UserID         string   `json:"user_id"`
	Name           string   `json:"name"`
	Avatar         string   `json:"avatar,omitempty"`
	Description    string   `json:"description"`
	Model          string   `json:"model"`
	SystemPrompt   string   `json:"system_prompt"`
	Temperature    float64  `json:"temperature"`
	TemperatureSet bool     `json:"temperature_set,omitempty"` // true if user explicitly set temperature
	MaxTokens      int      `json:"max_tokens"`
	MaxRounds      int      `json:"max_rounds"`
	Tools          []string `json:"tools"`
	Builtin        bool     `json:"builtin,omitempty"` // system-wide
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

type AgentStore struct {
	path   string
	mu     sync.RWMutex
	agents map[string]*AgentConfig
}

func NewAgentStore(path string) *AgentStore {
	s := &AgentStore{path: path, agents: make(map[string]*AgentConfig)}
	os.MkdirAll(filepath.Dir(path), 0o755)
	s.load()
	return s
}

func (s *AgentStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var agents []*AgentConfig
	if err := json.Unmarshal(data, &agents); err != nil {
		log.Printf("AgentStore load error: %v", err)
		return
	}
	for _, a := range agents {
		s.agents[a.ID] = a
	}
}

func (s *AgentStore) save() {
	list := make([]*AgentConfig, 0, len(s.agents))
	for _, a := range s.agents {
		list = append(list, a)
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("AgentStore save error: %v", err)
	}
}

func (s *AgentStore) ListByUser(userID string) []*AgentConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*AgentConfig, 0)
	for _, a := range s.agents {
		if a.UserID == userID || a.Builtin {
			result = append(result, a)
		}
	}
	return result
}

func (s *AgentStore) Get(id string) *AgentConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.agents[id]
}

func (s *AgentStore) Create(a *AgentConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.ID == "" {
		a.ID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	now := time.Now().Format(time.RFC3339)
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.MaxRounds == 0 {
		a.MaxRounds = 10
	}
	if !a.TemperatureSet && a.Temperature == 0 {
		a.Temperature = 0.7
	}
	if a.MaxTokens == 0 {
		a.MaxTokens = 4096
	}
	s.agents[a.ID] = a
	s.save()
	return nil
}

func (s *AgentStore) Update(id string, a *AgentConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.agents[id]
	if !ok {
		return errors.New("not found")
	}
	a.ID = id
	a.UserID = existing.UserID
	a.Builtin = existing.Builtin
	a.CreatedAt = existing.CreatedAt
	a.UpdatedAt = time.Now().Format(time.RFC3339)
	if a.MaxRounds == 0 {
		a.MaxRounds = existing.MaxRounds
	}
	if !a.TemperatureSet && a.Temperature == 0 {
		a.Temperature = existing.Temperature
		a.TemperatureSet = existing.TemperatureSet
	}
	if a.MaxTokens == 0 {
		a.MaxTokens = existing.MaxTokens
	}
	s.agents[id] = a
	s.save()
	return nil
}

func (s *AgentStore) Delete(id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[id]
	if !ok {
		return errors.New("not found")
	}
	if a.Builtin {
		return errors.New("cannot delete builtin agent")
	}
	if a.UserID != userID {
		return errors.New("forbidden")
	}
	delete(s.agents, id)
	s.save()
	return nil
}

// ════════════════════════════════════════════════
// Session metadata

type SessionMeta struct {
	ID        string   `json:"id"`
	UserID    string   `json:"user_id"`
	Title     string   `json:"title"`
	AgentIDs  []string `json:"agent_ids"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// ════════════════════════════════════════════════
// Tool registry

type ToolHandler struct {
	Schema  types.ToolSchema
	Handler func(types.ToolCall) types.ToolResult
}

// ════════════════════════════════════════════════
// Model Provider

type ModelProvider struct {
	ID        string   `json:"id"`
	UserID    string   `json:"user_id"`
	Name      string   `json:"name"`
	BaseURL   string   `json:"base_url"`
	APIKey    string   `json:"api_key"`
	Models    []string `json:"models"`
	Default   string   `json:"default"`
	Builtin   bool     `json:"builtin"`
	CreatedAt string   `json:"created_at"`
}

type ModelProviderStore struct {
	path      string
	mu        sync.RWMutex
	providers map[string]*ModelProvider
}

func NewModelProviderStore(path string) *ModelProviderStore {
	s := &ModelProviderStore{path: path, providers: make(map[string]*ModelProvider)}
	os.MkdirAll(filepath.Dir(path), 0o755)
	s.load()
	return s
}

func (s *ModelProviderStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var items []*ModelProvider
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("ModelProviderStore load error: %v", err)
		return
	}
	for _, p := range items {
		s.providers[p.ID] = p
	}
}

func (s *ModelProviderStore) save() {
	list := make([]*ModelProvider, 0, len(s.providers))
	for _, p := range s.providers {
		list = append(list, p)
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("ModelProviderStore save error: %v", err)
	}
}

func (s *ModelProviderStore) ListByUser(userID string) []*ModelProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*ModelProvider, 0)
	for _, p := range s.providers {
		if p.UserID == userID || p.Builtin {
			result = append(result, p)
		}
	}
	return result
}

func (s *ModelProviderStore) Get(id string) *ModelProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providers[id]
}

func (s *ModelProviderStore) Create(p *ModelProvider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = fmt.Sprintf("provider-%d", time.Now().UnixNano())
	}
	p.CreatedAt = time.Now().Format(time.RFC3339)
	s.providers[p.ID] = p
	s.save()
	return nil
}

func (s *ModelProviderStore) Update(id string, p *ModelProvider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.providers[id]
	if !ok {
		return errors.New("not found")
	}
	p.ID = id
	p.UserID = existing.UserID
	p.Builtin = existing.Builtin
	p.CreatedAt = existing.CreatedAt
	s.providers[id] = p
	s.save()
	return nil
}

func (s *ModelProviderStore) Delete(id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.providers[id]
	if !ok {
		return errors.New("not found")
	}
	if p.Builtin {
		return errors.New("cannot delete builtin provider")
	}
	if p.UserID != userID {
		return errors.New("forbidden")
	}
	delete(s.providers, id)
	s.save()
	return nil
}

// ════════════════════════════════════════════════
// User Skill

type UserSkill struct {
	ID          string            `json:"id"`
	UserID      string            `json:"user_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	Prompt      string            `json:"prompt"`
	Tools       []string          `json:"tools,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty"`
	Trigger     string            `json:"trigger,omitempty"`
	Builtin     bool              `json:"builtin"`
	CreatedBy   string            `json:"created_by,omitempty"` // "user" or "agent"
	UseCount    int               `json:"use_count"`
	LastUsedAt  string            `json:"last_used_at,omitempty"`
	Pinned      bool              `json:"pinned"`
	Archived    bool              `json:"archived"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type SkillStore struct {
	path   string
	mu     sync.RWMutex
	skills map[string]*UserSkill
}

func NewSkillStore(path string) *SkillStore {
	s := &SkillStore{path: path, skills: make(map[string]*UserSkill)}
	os.MkdirAll(filepath.Dir(path), 0o755)
	s.load()
	return s
}

func (s *SkillStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var items []*UserSkill
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("SkillStore load error: %v", err)
		return
	}
	for _, sk := range items {
		s.skills[sk.ID] = sk
	}
}

func (s *SkillStore) save() {
	list := make([]*UserSkill, 0, len(s.skills))
	for _, sk := range s.skills {
		list = append(list, sk)
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("SkillStore save error: %v", err)
	}
}

func (s *SkillStore) ListByUser(userID string) []*UserSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*UserSkill, 0)
	for _, sk := range s.skills {
		if sk.UserID == userID || sk.Builtin {
			result = append(result, sk)
		}
	}
	return result
}

func (s *SkillStore) ListBuiltin() []*UserSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*UserSkill, 0)
	for _, sk := range s.skills {
		if sk.Builtin {
			result = append(result, sk)
		}
	}
	return result
}

func (s *SkillStore) Get(id string) *UserSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skills[id]
}

func (s *SkillStore) Create(sk *UserSkill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sk.ID == "" {
		sk.ID = fmt.Sprintf("skill-%d", time.Now().UnixNano())
	}
	now := time.Now().Format(time.RFC3339)
	sk.CreatedAt = now
	sk.UpdatedAt = now
	s.skills[sk.ID] = sk
	s.save()
	return nil
}

func (s *SkillStore) Update(id string, sk *UserSkill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.skills[id]
	if !ok {
		return errors.New("not found")
	}
	sk.ID = id
	sk.UserID = existing.UserID
	sk.Builtin = existing.Builtin
	sk.CreatedAt = existing.CreatedAt
	sk.UpdatedAt = time.Now().Format(time.RFC3339)
	s.skills[id] = sk
	s.save()
	return nil
}

func (s *SkillStore) Delete(id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, ok := s.skills[id]
	if !ok {
		return errors.New("not found")
	}
	if sk.Builtin {
		return errors.New("cannot delete builtin skill")
	}
	if sk.UserID != userID {
		return errors.New("forbidden")
	}
	delete(s.skills, id)
	s.save()
	return nil
}

// Curator methods

func (s *SkillStore) RecordUsage(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, ok := s.skills[id]
	if !ok {
		return
	}
	sk.UseCount++
	sk.LastUsedAt = time.Now().Format(time.RFC3339)
	sk.UpdatedAt = sk.LastUsedAt
	s.save()
}

func (s *SkillStore) Pin(id string, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, ok := s.skills[id]
	if !ok {
		return errors.New("not found")
	}
	sk.Pinned = pinned
	sk.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return nil
}

func (s *SkillStore) Archive(id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, ok := s.skills[id]
	if !ok {
		return errors.New("not found")
	}
	sk.Archived = archived
	sk.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return nil
}

func (s *SkillStore) RunCurator(staleDays int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	archivedCount := 0
	for _, sk := range s.skills {
		// Skip builtin, pinned, already archived, or user-created skills
		if sk.Builtin || sk.Pinned || sk.Archived || sk.CreatedBy != "agent" {
			continue
		}
		// Check if stale
		if sk.LastUsedAt == "" {
			sk.LastUsedAt = sk.CreatedAt
		}
		lastUsed, err := time.Parse(time.RFC3339, sk.LastUsedAt)
		if err != nil {
			continue
		}
		if now.Sub(lastUsed) > time.Duration(staleDays)*24*time.Hour {
			sk.Archived = true
			sk.UpdatedAt = now.Format(time.RFC3339)
			archivedCount++
			log.Printf("[CURATOR] Archived stale skill: %s (last used: %s)", sk.Name, sk.LastUsedAt)
		}
	}
	if archivedCount > 0 {
		s.save()
	}
	return archivedCount
}

func (s *SkillStore) CuratorStatus() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	agentCreated := 0
	pinned := 0
	archived := 0
	for _, sk := range s.skills {
		if sk.Builtin {
			continue
		}
		total++
		if sk.CreatedBy == "agent" {
			agentCreated++
		}
		if sk.Pinned {
			pinned++
		}
		if sk.Archived {
			archived++
		}
	}
	return map[string]any{
		"total":         total,
		"agent_created": agentCreated,
		"pinned":        pinned,
		"archived":      archived,
	}
}

// ════════════════════════════════════════════════
// Invite Code

type InviteCode struct {
	Code      string `json:"code"`
	CreatedBy string `json:"created_by"`
	UsedBy    []string `json:"used_by"`
	MaxUses   int    `json:"max_uses"`
	UseCount  int    `json:"use_count"`
	CreatedAt string `json:"created_at"`
}

func generateInviteCode() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	code := make([]byte, 8)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		code[i] = chars[n.Int64()]
	}
	return string(code)
}

// ════════════════════════════════════════════════
// Team Store

type TeamMember struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

type Team struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	CoordinatorID string       `json:"coordinator_id"`
	Members       []TeamMember `json:"members"`
	UserID        string       `json:"user_id"`
	Builtin       bool         `json:"builtin"`
	CreatedAt     string       `json:"created_at"`
}

func (t *Team) MemberNames() string {
	names := make([]string, len(t.Members))
	for i, m := range t.Members {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}

// ════════════════════════════════════════════════
// Chat Engine

type ChatEngine struct {
	defaultProvider    llm.Provider
	apiKey             string
	baseURL            string
	store              *store.SQLite
	appDB              *sql.DB            // App-level SQLite (users, sessions, teams, etc.)
	userStore          *UserStore
	agentStore         *AgentStore
	sessionStore       *SessionStore
	providerStore      *ModelProviderStore
	skillStore         *SkillStore
	runtimeSkillEngine *SkillEngine
	inviteCodeStore    *InviteCodeStore
	teamStore          *TeamStore
	memoryStore        *MemoryStore
	memorySkill        *memory.Skill  // Persistent memory
	cronSkill          *CronSkill     // Scheduled jobs
	tools              map[string]ToolHandler
	allSchemas         []types.ToolSchema
	activeCancels      map[string]context.CancelFunc // sessionID -> cancel
	activeCancelsMu    sync.Mutex
}

func NewChatEngine() (*ChatEngine, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is required")
	}
	baseURL := envOr("OPENAI_BASE_URL", "https://api.openai.com/v1")
	model := envOr("OPENAI_MODEL", "gpt-4o")

	provider := llm.NewOpenAI(apiKey, baseURL, model)

	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".crux-chat")

	dbPath := filepath.Join(dataDir, "chat.db")
	s, err := store.NewSQLite(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	// Open app-level DB for users/sessions/teams/etc.
	appDB, err := openAppDB(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open app db: %w", err)
	}
	// Migrate JSON data to DB (one-time, no-op if already done)
	migrateJSONToDB(appDB, dataDir)

	engine := &ChatEngine{
		defaultProvider:    provider,
		apiKey:             apiKey,
		baseURL:            baseURL,
		store:              s,
		appDB:              appDB,
		userStore:          NewUserStore(appDB),
		agentStore:         NewAgentStore(filepath.Join(dataDir, "agents.json")),
		sessionStore:       NewSessionStore(appDB),
		providerStore:      NewModelProviderStore(filepath.Join(dataDir, "model_providers.json")),
		skillStore:         NewSkillStore(filepath.Join(dataDir, "skills.json")),
		runtimeSkillEngine: NewSkillEngine(provider, appDB),
		inviteCodeStore:    NewInviteCodeStore(appDB),
		teamStore:          NewTeamStore(appDB),
		memoryStore:        NewMemoryStore(appDB),
		tools:              make(map[string]ToolHandler),
		activeCancels:      make(map[string]context.CancelFunc),
	}

	// Start runtime skill cleanup loop
	go engine.runtimeSkillEngine.StartCleanupLoop(context.Background())

	// Start curator loop (runs daily, archives skills unused for 30+ days)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			count := engine.skillStore.RunCurator(30)
			if count > 0 {
				log.Printf("[CURATOR] Archived %d stale skills", count)
			}
		}
	}()

	// Register tools
	workDir, _ := os.Getwd()
	fp := primitives.NewFileSandbox([]string{workDir, home, "/tmp"})
	engine.registerToolSet(fp.ToolSchemas(), fp.Handle)
	sp := primitives.NewSystem()
	engine.registerToolSet(sp.ToolSchemas(), sp.Handle)
	ts := terminal.New()
	ts.Timeout = 30 * time.Second
	ts.WorkDir = workDir
	engine.registerToolSet(ts.ToolSchemas(), ts.Handle)
	memDir := filepath.Join(dataDir, "memory")
	mem := memory.New(memDir)
	mem.Init(nil)
	engine.registerToolSet(mem.ToolSchemas(), mem.Handle)
	engine.memorySkill = mem

	// Register web search skill (web_search + fetch_url)
	webSearch := NewWebSearchSkill()
	engine.registerMapToolSet(webSearch.ToolSchemas(), webSearch.Handle)

	// Register cron skill (cron_add + cron_list + cron_remove)
	cronSkill := NewCronSkill()
	cronSkill.SetEngine(engine)
	engine.registerMapToolSet(cronSkill.ToolSchemas(), cronSkill.Handle)
	engine.cronSkill = cronSkill

	// Register skill management tools (skill_manage + skill_list)
	skillMgmt := NewSkillManageTool()
	skillMgmt.SetEngine(engine)
	engine.registerMapToolSet(skillMgmt.ToolSchemas(), skillMgmt.Handle)

	// Register context compaction tool (compact_messages)
	compaction := NewCompactionSkill()
	compaction.SetEngine(engine)
	engine.registerMapToolSet(compaction.ToolSchemas(), compaction.Handle)

	// Seed builtin agent if none
	hasBuiltin := false
	for _, a := range engine.agentStore.agents {
		if a.Builtin {
			hasBuiltin = true
			break
		}
	}
	if !hasBuiltin {
		engine.agentStore.Create(&AgentConfig{
			ID:      "default",
			Name:    "Crux",
			Description: "通用 AI 助手，可使用所有工具",
			Model:   model,
			SystemPrompt: `你是 益枢(Crux)，一个有用的 AI 助手。你可以使用工具帮助用户完成各种任务。请简洁准确地回答。

你拥有以下能力：
- 文件操作：read_file, write_file, patch_file, search_content, search_files
- 系统信息：get_time, get_os, get_cwd, get_env
- 命令执行：exec（运行shell命令）
- 网络搜索：web_search（搜索网页）, fetch_url（抓取网页内容）
- 持久记忆：memory_save, memory_search, memory_list, memory_delete
- 定时任务：cron_add, cron_list, cron_remove
- 技能管理：skill_manage, skill_list
- 上下文压缩：compact_messages

当学到重要信息时，使用 memory_save 保存记忆。当完成复杂任务后，考虑用 skill_manage 沉淀可复用的技能。`,
			Tools:   engine.toolNames(),
			Builtin: true,
		})
	}

	// Seed builtin model provider if none
	hasBuiltinProvider := false
	for _, p := range engine.providerStore.providers {
		if p.Builtin {
			hasBuiltinProvider = true
			break
		}
	}
	if !hasBuiltinProvider {
		engine.providerStore.Create(&ModelProvider{
			ID:      "default",
			Name:    "OpenAI",
			BaseURL: baseURL,
			APIKey:  apiKey,
			Models:  []string{model},
			Default: model,
			Builtin: true,
		})
	}

	// Seed builtin skills if none
	hasBuiltinSkill := false
	for _, sk := range engine.skillStore.skills {
		if sk.Builtin {
			hasBuiltinSkill = true
			break
		}
	}
	if !hasBuiltinSkill {
		engine.skillStore.Create(&UserSkill{
			ID:          "terminal",
			Name:        "Terminal",
			Description: "Execute shell commands on the system",
			Icon:        "🖥️",
			Prompt:      "You have access to a terminal. Use the exec tool to execute shell commands when the user asks for system operations, file manipulation, or running scripts.",
			Tools:       []string{"exec"},
			Builtin:     true,
		})
		engine.skillStore.Create(&UserSkill{
			ID:          "web-search",
			Name:        "Web Search",
			Description: "Search the web and fetch URLs for up-to-date information",
			Icon:        "🔍",
			Prompt:      "You can search the web for up-to-date information. Use web_search tool when the user asks about current events, needs real-time data, or when your knowledge might be outdated. Use fetch_url to read the content of a specific URL.",
			Tools:       []string{"web_search", "fetch_url"},
			Builtin:     true,
		})
		engine.skillStore.Create(&UserSkill{
			ID:          "cron",
			Name:        "Scheduled Tasks",
			Description: "Create and manage scheduled background tasks",
			Icon:        "⏰",
			Prompt:      "You can schedule tasks to run periodically in the background. Use cron_add to create a job, cron_list to see all jobs, cron_remove to delete one.",
			Tools:       []string{"cron_add", "cron_list", "cron_remove"},
			Builtin:     true,
		})
		engine.skillStore.Create(&UserSkill{
			ID:          "skill-management",
			Name:        "Skill Management",
			Description: "Create, manage, and evolve reusable skills",
			Icon:        "🧩",
			Prompt:      "You can create and manage reusable skills. Use skill_manage to create/update/archive/pin skills. Use skill_list to browse available skills. After completing complex tasks (5+ tool calls), consider creating a skill to remember the workflow.",
			Tools:       []string{"skill_manage", "skill_list"},
			Builtin:     true,
		})
		engine.skillStore.Create(&UserSkill{
			ID:          "context-compaction",
			Name:        "Context Compaction",
			Description: "Compress long conversation contexts",
			Icon:        "📦",
			Prompt:      "If the conversation is getting very long and you're losing context, use compact_messages to compress older messages into a summary.",
			Tools:       []string{"compact_messages"},
			Builtin:     true,
		})
	}

	log.Printf("Crux Chat: %d tools, %d agents, %d users",
		len(engine.allSchemas), len(engine.agentStore.agents), len(engine.userStore.List()))
	return engine, nil
}

func (e *ChatEngine) registerToolSet(schemas []types.ToolSchema, handler func(types.ToolCall) types.ToolResult) {
	for _, s := range schemas {
		e.allSchemas = append(e.allSchemas, s)
		e.tools[s.Name] = ToolHandler{Schema: s, Handler: handler}
	}
}

// registerMapToolSet registers tools that use map[string]interface{} schemas (OpenAI format).
// This bridges the new skill interface with the existing ToolSchema-based system.
func (e *ChatEngine) registerMapToolSet(schemas []map[string]interface{}, handler func(name string, args map[string]interface{}) (string, error)) {
	for _, schemaMap := range schemas {
		fn, ok := schemaMap["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)

		schema := types.ToolSchema{
			Name:        name,
			Description: desc,
			Parameters:  params,
		}
		e.allSchemas = append(e.allSchemas, schema)

		// Capture name and handler for closure
		toolName := name
		toolHandler := handler
		e.tools[name] = ToolHandler{
			Schema: schema,
			Handler: func(tc types.ToolCall) types.ToolResult {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					return types.ToolResult{ToolCallID: tc.ID, Content: fmt.Sprintf("Error: invalid arguments: %v", err), IsError: true}
				}
				content, err := toolHandler(toolName, args)
				if err != nil {
					return types.ToolResult{ToolCallID: tc.ID, Content: fmt.Sprintf("Error: %v", err), IsError: true}
				}
				return types.ToolResult{ToolCallID: tc.ID, Content: content}
			},
		}
	}
}

func (e *ChatEngine) toolNames() []string {
	names := make([]string, 0, len(e.tools))
	for name := range e.tools {
		names = append(names, name)
	}
	return names
}

func (e *ChatEngine) getProvider(model string, userID string) llm.Provider {
	if model == "" {
		return e.defaultProvider
	}

	// Check if model is in providerID/modelName format
	if idx := strings.Index(model, "/"); idx > 0 {
		providerID := model[:idx]
		modelName := model[idx+1:]
		// Look up in provider store (user's own + builtins)
		for _, p := range e.providerStore.ListByUser(userID) {
			if p.ID == providerID {
				return llm.NewOpenAI(p.APIKey, p.BaseURL, modelName)
			}
		}
	}

	if model == e.defaultProvider.Model() {
		return e.defaultProvider
	}
	return llm.NewOpenAI(e.apiKey, e.baseURL, model)
}

func (e *ChatEngine) getSchemas(toolNames []string) []types.ToolSchema {
	if len(toolNames) == 0 {
		return e.allSchemas
	}
	nameSet := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		nameSet[n] = true
	}
	var schemas []types.ToolSchema
	for _, s := range e.allSchemas {
		if nameSet[s.Name] {
			schemas = append(schemas, s)
		}
	}
	return schemas
}

func (e *ChatEngine) executeTool(tc types.ToolCall, extraTools ...map[string]ToolHandler) types.ToolResult {
	var et map[string]ToolHandler
	if len(extraTools) > 0 {
		et = extraTools[0]
	}
	if et != nil {
		if th, ok := et[tc.Function.Name]; ok {
			return th.Handler(tc)
		}
	}
	th, ok := e.tools[tc.Function.Name]
	if !ok {
		return types.ToolResult{ToolCallID: tc.ID, Content: fmt.Sprintf("Unknown tool: %s", tc.Function.Name)}
	}
	return th.Handler(tc)
}

// ════════════════════════════════════════════════
// API types

type ChatMessage struct {
	Role    string        `json:"role"`
	Content string        `json:"content"`
	Name    string        `json:"name,omitempty"`
	Images  []ImageAttach `json:"images,omitempty"`
}

type ImageAttach struct {
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages" binding:"required"`
	AgentID  string        `json:"agent_id,omitempty"`
	AgentIDs []string      `json:"agent_ids,omitempty"`
	Session  string        `json:"session,omitempty"`
	SkillIDs []string      `json:"skill_ids,omitempty"`
	TeamID   string        `json:"team_id,omitempty"`
}

type MessageResp struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	Name       string `json:"name,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// ════════════════════════════════════════════════
// Auth middleware

func authMiddleware(userStore *UserStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		claims, err := parseToken(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		user := userStore.GetByID(claims.UserID)
		if user == nil {
			c.JSON(401, gin.H{"error": "user not found"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Set("userID", user.ID)
		c.Next()
	}
}

func optionalAuth(userStore *UserStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth != "" && strings.HasPrefix(auth, "Bearer ") {
			claims, err := parseToken(strings.TrimPrefix(auth, "Bearer "))
			if err == nil {
				user := userStore.GetByID(claims.UserID)
				if user != nil {
					c.Set("user", user)
					c.Set("userID", user.ID)
				}
			}
		}
		c.Next()
	}
}

// ════════════════════════════════════════════════
// Main

func main() {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".crux-chat")

	engine, err := NewChatEngine()
	if err != nil {
		log.Fatalf("Init failed: %v", err)
	}

	r := gin.Default()
	corsOrigins := envOr("CORS_ORIGINS", "")
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	if corsOrigins == "" || corsOrigins == "*" {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = strings.Split(corsOrigins, ",")
	}
	r.Use(cors.New(corsConfig))

	api := r.Group("/api")

	// Upload static files
	os.MkdirAll(uploadDir, 0o755)
	r.Static("/uploads", uploadDir)

	// ── Public routes ──
	api.POST("/auth/register", engine.Register)
	api.POST("/auth/login", engine.Login)
	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"model":  engine.defaultProvider.Model(),
			"engine": "crux-go",
			"tools":  len(engine.allSchemas),
		})
	})

	// ── Protected routes ──
	auth := api.Group("")
	auth.Use(authMiddleware(engine.userStore))
	{
		// Upload (requires auth)
		auth.POST("/upload", handleUpload)

		// User
		auth.GET("/auth/me", engine.GetMe)
		auth.PUT("/auth/profile", engine.UpdateProfile)

		// Invite codes
		auth.GET("/invite-code", engine.GetMyInviteCode)

		// Chat
		auth.POST("/chat", engine.HandleChat)
		auth.POST("/chat/cancel", engine.HandleChatCancel)

		// Sessions
		auth.GET("/sessions", engine.ListSessions)
		auth.POST("/sessions", engine.CreateSession)
		auth.GET("/sessions/:id/messages", engine.GetSessionMessages)
		auth.PUT("/sessions/:id", engine.UpdateSession)
		auth.DELETE("/sessions/:id", engine.DeleteSession)

		// Agents
		auth.GET("/agents", engine.ListAgents)
		auth.POST("/agents", engine.CreateAgent)
		auth.GET("/agents/:id", engine.GetAgent)
		auth.PUT("/agents/:id", engine.UpdateAgent)

		// Teams
		auth.GET("/teams", engine.ListTeams)
		auth.POST("/teams", engine.CreateTeam)
		auth.GET("/teams/:id", engine.GetTeam)
		auth.PUT("/teams/:id", engine.UpdateTeam)
		auth.DELETE("/teams/:id", engine.DeleteTeam)

		auth.GET("/tools", engine.ListTools)

		// Models
		auth.GET("/models/available", engine.ListAvailableModels)

		// User config - Model Providers
		auth.GET("/user/providers", engine.ListProviders)
		auth.POST("/user/providers", engine.CreateProvider)
		auth.PUT("/user/providers/:id", engine.UpdateProvider)
		auth.DELETE("/user/providers/:id", engine.DeleteProvider)

		// Skills
		auth.GET("/skills", engine.ListSkills)
		auth.POST("/skills", engine.CreateSkill)
		auth.PUT("/skills/:id", engine.UpdateSkill)
		auth.DELETE("/skills/:id", engine.DeleteSkill)
		auth.POST("/admin/skills", engine.CreateBuiltinSkill)

		// Curator endpoints
		auth.GET("/curator/status", engine.CuratorStatus)
		auth.POST("/curator/run", engine.RunCurator)
		auth.POST("/skills/:id/pin", engine.PinSkill)
		auth.POST("/skills/:id/archive", engine.ArchiveSkill)

		// Runtime Skills (auto-generated by skill engine)
		auth.GET("/runtime-skills", engine.ListRuntimeSkills)
		auth.GET("/runtime-skills/stats", engine.SkillStats)
		auth.GET("/runtime-skills/:id", engine.GetRuntimeSkill)
		auth.PUT("/runtime-skills/:id", engine.UpdateRuntimeSkill)
		auth.DELETE("/runtime-skills/:id", engine.DeleteRuntimeSkill)
		auth.POST("/runtime-skills/:id/pin", engine.PinRuntimeSkill)
		auth.POST("/runtime-skills/:id/archive", engine.ArchiveRuntimeSkill)
	}

	// ── Gateway (multi-platform messaging, configurable from web UI) ──
	_ = dataDir
	gwConfig := NewGatewayConfigStore(filepath.Join(dataDir, "gateway_config.json"))
	gwManager := NewGatewayManager(engine, gwConfig, filepath.Join(dataDir, "gateway_sessions.json"))

	// Webhook routes (public, no auth)
	gwAPI := api.Group("/gateway")
	gwAPI.POST("/feishu/webhook", gwManager.HandleFeishuWebhook)

	// Gateway management API (protected)
	auth.GET("/gateway/config", gwManager.ListConfig)
	auth.GET("/gateway/config/:platform", gwManager.GetConfig)
	auth.PUT("/gateway/config/:platform", gwManager.SetConfig)
	auth.DELETE("/gateway/config/:platform", gwManager.DeleteConfig)
	auth.POST("/gateway/:platform/start", gwManager.HandleStartPlatform)
	auth.POST("/gateway/:platform/stop", gwManager.HandleStopPlatform)
	auth.GET("/gateway/status", gwManager.HandleStatus)
	auth.GET("/gateway/wechat/qr", gwManager.GetWeChatQR)
	auth.GET("/gateway/wechat/qr-status", gwManager.GetWeChatQRStatus)

	// Auto-start enabled platforms
	gwManager.Start()

	log.Printf("Crux Chat on %s (model: %s, tools: %d)",
		listenAddr, engine.defaultProvider.Model(), len(engine.allSchemas))
	r.Run(listenAddr)
}

// ════════════════════════════════════════════════
// Auth handlers

func (e *ChatEngine) Register(c *gin.Context) {
	// Reject if already authenticated
	auth := c.GetHeader("Authorization")
	if auth != "" && strings.HasPrefix(auth, "Bearer ") {
		if _, err := parseToken(strings.TrimPrefix(auth, "Bearer ")); err == nil {
			c.JSON(400, gin.H{"error": "already logged in"})
			return
		}
	}
	var req struct {
		Username   string `json:"username" binding:"required"`
		Password   string `json:"password" binding:"required"`
		Nickname   string `json:"nickname"`
		InviteCode string `json:"invite_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if len(req.Username) < 3 {
		c.JSON(400, gin.H{"error": "username must be at least 3 characters"})
		return
	}
	if len(req.Password) < 6 {
		c.JSON(400, gin.H{"error": "password must be at least 6 characters"})
		return
	}
	if req.Nickname == "" {
		req.Nickname = req.Username
	}

	// Invite code validation
	if req.InviteCode != "" {
		if err := e.inviteCodeStore.ValidateAndUse(req.InviteCode, ""); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
	} else if len(e.userStore.List()) > 0 {
		c.JSON(400, gin.H{"error": "invite code is required"})
		return
	}

	user, err := e.userStore.Create(req.Username, req.Password, req.Nickname)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Update invite code with actual user ID
	if req.InviteCode != "" {
		e.inviteCodeStore.SetUsedBy(req.InviteCode, user.ID)
	}

	token, err := generateToken(user)
	if err != nil {
		c.JSON(500, gin.H{"error": "token generation failed"})
		return
	}

	user.Password = ""
	c.JSON(200, gin.H{"user": user, "token": token})
}

func (e *ChatEngine) Login(c *gin.Context) {
	// Reject if already authenticated
	auth := c.GetHeader("Authorization")
	if auth != "" && strings.HasPrefix(auth, "Bearer ") {
		if _, err := parseToken(strings.TrimPrefix(auth, "Bearer ")); err == nil {
			c.JSON(400, gin.H{"error": "already logged in"})
			return
		}
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user, err := e.userStore.Authenticate(req.Username, req.Password)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := generateToken(user)
	if err != nil {
		c.JSON(500, gin.H{"error": "token generation failed"})
		return
	}

	user.Password = ""
	c.JSON(200, gin.H{"user": user, "token": token})
}

func (e *ChatEngine) GetMe(c *gin.Context) {
	user := c.MustGet("user").(*User)
	user.Password = ""
	c.JSON(200, gin.H{"user": user})
}

func (e *ChatEngine) UpdateProfile(c *gin.Context) {
	user := c.MustGet("user").(*User)
	var req struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	e.userStore.Update(user.ID, req.Nickname, req.Avatar)
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) ListUsers(c *gin.Context) {
	user := c.MustGet("user").(*User)
	if user.Role != "admin" {
		c.JSON(403, gin.H{"error": "admin only"})
		return
	}
	c.JSON(200, gin.H{"users": e.userStore.List()})
}

func (e *ChatEngine) GetMyInviteCode(c *gin.Context) {
	userID := c.GetString("userID")
	ic := e.inviteCodeStore.GetByUser(userID)
	c.JSON(200, gin.H{"invite_code": ic})
}

func (e *ChatEngine) ListMyInviteCodes(c *gin.Context) {
	userID := c.GetString("userID")
	codes := e.inviteCodeStore.ListByUser(userID)
	c.JSON(200, gin.H{"invite_codes": codes})
}

// ════════════════════════════════════════════════
// Session handlers (user-scoped)

func (e *ChatEngine) ListSessions(c *gin.Context) {
	userID := c.GetString("userID")
	c.JSON(200, gin.H{"sessions": e.sessionStore.ListByUser(userID)})
}

func (e *ChatEngine) CreateSession(c *gin.Context) {
	userID := c.GetString("userID")
	var body struct {
		AgentID string `json:"agent_id"`
		Title   string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Title == "" {
		body.Title = "新对话"
	}
	agentIDs := []string{body.AgentID}
	if body.AgentID == "" {
		agentIDs = []string{}
	}
	sess := &SessionMeta{
		ID:       fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		UserID:   userID,
		Title:    body.Title,
		AgentIDs: agentIDs,
	}
	e.sessionStore.Create(sess)
	c.JSON(200, gin.H{"session": sess})
}

func (e *ChatEngine) GetSessionMessages(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("userID")
	sess := e.sessionStore.Get(id)
	if sess == nil || sess.UserID != userID {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	msgs, err := e.store.GetMessages(id, store.MessageOpts{Limit: 500})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	result := make([]MessageResp, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, MessageResp{
			ID: m.ID, Role: m.Role, Content: m.Content, Name: m.Name,
			ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID, CreatedAt: m.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(200, gin.H{"messages": result, "session": sess})
}

func (e *ChatEngine) UpdateSession(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("userID")
	sess := e.sessionStore.Get(id)
	if sess == nil || sess.UserID != userID {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	e.sessionStore.UpdateTitle(id, body.Title)
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("userID")
	if err := e.sessionStore.Delete(id, userID); err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	// Also delete messages from SQLite (CASCADE handles it, but be explicit)
	if err := e.store.DeleteSession(id); err != nil {
		log.Printf("Warning: failed to delete session messages from SQLite: %v", err)
	}
	c.JSON(200, gin.H{"ok": true})
}

// ════════════════════════════════════════════════
// Agent CRUD (user-scoped + builtin)

func (e *ChatEngine) ListAgents(c *gin.Context) {
	userID := c.GetString("userID")
	c.JSON(200, gin.H{"agents": e.agentStore.ListByUser(userID)})
}

func (e *ChatEngine) GetAgent(c *gin.Context) {
	agent := e.agentStore.Get(c.Param("id"))
	if agent == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, agent)
}

func (e *ChatEngine) CreateAgent(c *gin.Context) {
	userID := c.GetString("userID")
	var agent AgentConfig
	if err := c.ShouldBindJSON(&agent); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	agent.UserID = userID
	e.agentStore.Create(&agent)
	c.JSON(200, agent)
}

func (e *ChatEngine) UpdateAgent(c *gin.Context) {
	user := c.MustGet("user").(*User)
	userID := user.ID
	id := c.Param("id")
	existing := e.agentStore.Get(id)
	if existing == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if existing.Builtin && user.Role != "admin" {
		c.JSON(403, gin.H{"error": "cannot modify builtin agent"})
		return
	}
	if existing.UserID != userID && !existing.Builtin {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	var agent AgentConfig
	if err := c.ShouldBindJSON(&agent); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	e.agentStore.Update(id, &agent)
	c.JSON(200, agent)
}

func (e *ChatEngine) DeleteAgent(c *gin.Context) {
	userID := c.GetString("userID")
	if err := e.agentStore.Delete(c.Param("id"), userID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) ListTools(c *gin.Context) {
	tools := make([]map[string]string, 0, len(e.allSchemas))
	for _, s := range e.allSchemas {
		tools = append(tools, map[string]string{"name": s.Name, "description": s.Description})
	}
	c.JSON(200, gin.H{"tools": tools})
}

// ════════════════════════════════════════════════
// Chat handler (user-scoped)

func (e *ChatEngine) HandleChat(c *gin.Context) {
	userID := c.GetString("userID")
	log.Printf("[CHAT] Request from user=%s", userID)

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[CHAT] Bind error: %v", err)
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[CHAT] Session=%s, Agents=%v, TeamID=%s, Messages=%d, Skills=%v",
		req.Session, req.AgentIDs, req.TeamID, len(req.Messages), req.SkillIDs)

	// Resolve agents
	agentIDs := req.AgentIDs
	if len(agentIDs) == 0 && req.AgentID != "" {
		agentIDs = []string{req.AgentID}
	}
	if len(agentIDs) == 0 {
		agentIDs = []string{"default"}
	}

	var agents []*AgentConfig
	for _, id := range agentIDs {
		a := e.agentStore.Get(id)
		if a == nil {
			c.JSON(400, gin.H{"error": fmt.Sprintf("agent not found: %s", id)})
			return
		}
		agents = append(agents, a)
	}

	// Session
	sessionID := req.Session
	isNewSession := false
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
		e.sessionStore.Create(&SessionMeta{
			ID:       sessionID,
			UserID:   userID,
			Title:    generateTitle(req.Messages),
			AgentIDs: agentIDs,
		})
		isNewSession = true
	} else {
		sess := e.sessionStore.Get(sessionID)
		if sess == nil || sess.UserID != userID {
			c.JSON(403, gin.H{"error": "session not found or forbidden"})
			return
		}
	}

	// Load history
	history, _ := e.store.GetMessages(sessionID, store.MessageOpts{Limit: 500})
	var messages []types.Message
	for _, m := range history {
		msg := types.Message{Role: m.Role, Content: m.Content, Name: m.Name}
		if len(m.ToolCalls) > 0 {
			json.Unmarshal(m.ToolCalls, &msg.ToolCalls)
		}
		msg.ToolCallID = m.ToolCallID
		messages = append(messages, msg)
	}
	log.Printf("[CHAT] Loaded %d history messages for session=%s", len(messages), sessionID)

	// Append only the NEW user message (last in req.Messages)
	// Frontend sends full conversation, but we already have history from DB
	if len(req.Messages) > 0 {
		m := req.Messages[len(req.Messages)-1]
		msg := types.Message{Role: m.Role, Content: m.Content, Name: m.Name}
		for _, img := range m.Images {
			data, mime, err := e.loadImage(img)
			if err != nil {
				continue
			}
			msg.Content += fmt.Sprintf("\n\n![image](data:%s;base64,%s)", mime, data)
		}
		messages = append(messages, msg)
		e.saveMsg(sessionID, msg)
	}

	// SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(500, gin.H{"error": "streaming not supported"})
		return
	}

	// Send session ID to frontend
	sseSend(c, flusher, "session", sessionID, false)

	// Send skill_activate if skills are provided
	if len(req.SkillIDs) > 0 {
		var skillInfos []map[string]string
		for _, sid := range req.SkillIDs {
			sk := e.skillStore.Get(sid)
			if sk != nil {
				skillInfos = append(skillInfos, map[string]string{"id": sk.ID, "name": sk.Name, "icon": sk.Icon})
			}
		}
		if len(skillInfos) > 0 {
			data, _ := json.Marshal(skillInfos)
			sseSend(c, flusher, "skill_activate", string(data), false)
		}
	}

	// Launch AI title generation for new sessions
	var titleWg sync.WaitGroup
	if isNewSession {
		firstMsg := ""
		for _, m := range req.Messages {
			if m.Role == "user" && m.Content != "" {
				firstMsg = m.Content
				break
			}
		}
		if firstMsg != "" {
			titleWg.Add(1)
			go e.generateAITitle(sessionID, firstMsg, agents[0].Model, userID, c, flusher, &titleWg)
		}
	}

	// Team mode
	if req.TeamID != "" {
		team := e.teamStore.Get(req.TeamID)
		if team == nil {
			sseSend(c, flusher, "error", "team not found", true)
			return
		}
		coordinator := e.agentStore.Get(team.CoordinatorID)
		if coordinator == nil {
			sseSend(c, flusher, "error", "coordinator agent not found", true)
			return
		}
		teamPrompt := e.buildTeamPrompt(team)
		delHandler := e.createDelegateHandler(team, c, flusher, userID)
		extraTools := map[string]ToolHandler{"delegate_task": delHandler}
		e.runAgentLoop(c, flusher, coordinator, messages, sessionID, nil, userID, extraTools, teamPrompt)
		titleWg.Wait()
		sseSend(c, flusher, "done", "", true)
		return
	}

	if len(agents) == 1 {
		e.runAgentLoop(c, flusher, agents[0], messages, sessionID, req.SkillIDs, userID, nil, "")
		titleWg.Wait()
		sseSend(c, flusher, "done", "", true)
		return
	}

	// Multi-agent
	for _, agent := range agents {
		sseSend(c, flusher, "agent_start", agent.ID, false)
		e.runAgentLoop(c, flusher, agent, messages, sessionID, req.SkillIDs, userID, nil, "")
		history, _ = e.store.GetMessages(sessionID, store.MessageOpts{Limit: 500})
		messages = nil
		for _, m := range history {
			// Skip orphaned tool messages when passing context to next agent
			if m.Role == "tool" {
				continue
			}
			msg := types.Message{Role: m.Role, Content: m.Content, Name: m.Name}
			if len(m.ToolCalls) > 0 {
				json.Unmarshal(m.ToolCalls, &msg.ToolCalls)
			}
			msg.ToolCallID = m.ToolCallID
			messages = append(messages, msg)
		}
		sseSend(c, flusher, "agent_end", agent.ID, false)
	}
	titleWg.Wait()
	sseSend(c, flusher, "done", "", true)
}

func (e *ChatEngine) runAgentLoop(c *gin.Context, flusher http.Flusher, agent *AgentConfig, messages []types.Message, sessionID string, skillIDs []string, userID string, extraTools map[string]ToolHandler, systemAppend string) {
	log.Printf("[AGENT] Starting agent=%s, session=%s, messages=%d, skills=%v", agent.Name, sessionID, len(messages), skillIDs)

	// Start tracking for runtime skill engine
	e.runtimeSkillEngine.tracker.StartSession(sessionID)
	defer func() {
		trace := e.runtimeSkillEngine.tracker.EndSession(sessionID)
		if trace != nil && len(trace.Calls) >= 5 {
			go e.runtimeSkillEngine.ProcessCompletedTrace(trace)
		}
	}()

	// Create a timeout context for the entire agent loop
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	// Register cancel func so /api/chat/cancel can abort this session
	e.activeCancelsMu.Lock()
	e.activeCancels[sessionID] = cancel
	e.activeCancelsMu.Unlock()
	defer func() {
		e.activeCancelsMu.Lock()
		delete(e.activeCancels, sessionID)
		e.activeCancelsMu.Unlock()
	}()

	provider := e.getProvider(agent.Model, userID)
	schemas := e.getSchemas(agent.Tools)
	for _, th := range extraTools {
		schemas = append(schemas, th.Schema)
	}
	maxRounds := agent.MaxRounds
	if maxRounds == 0 {
		maxRounds = 10
	}

	// Build system prompt with skill prompts and memory
	systemContent := agent.SystemPrompt

	// Inject memory context
	if e.memorySkill != nil {
		memText := e.memorySkill.GetMemoryText()
		if memText != "" {
			systemContent += "\n\n## Your Memory\n" + memText
		}
	}

	// Add memory and skill management instructions
	systemContent += `

## Memory & Skill Management
You have persistent memory across sessions. Use these tools to remember important information:

**When to save memories (memory_save):**
- User preferences, habits, or personal details
- Environment details (OS, tools, paths, configs)
- Tool quirks, API behaviors, or workarounds you discovered
- Corrections the user gives you
- Stable conventions that will be useful in future sessions

**When NOT to save:**
- Task progress, session outcomes, or temporary state
- Things that will be stale in a week
- Raw data dumps

**Format memories as declarative facts:**
- "User prefers concise responses" ✓
- "Always respond concisely" ✗ (imperative)

**When to create skills (skill_manage):**
- After complex tasks (5+ tool calls) that succeeded
- When you discover a reusable workflow
- When fixing a tricky error that others might encounter
- When the user corrects your approach and it works

**Curator:** Skills you create are tracked automatically. Unused skills get archived after 30 days. Pin important skills to protect them.`

	var skillEnvKeys []string
	if len(skillIDs) > 0 {
		for _, sid := range skillIDs {
			sk := e.skillStore.Get(sid)
			if sk == nil {
				continue
			}
			if sk.Prompt != "" {
				systemContent += "\n\n" + sk.Prompt
			}
		// Set env vars from skills (scoped to this request via process env)
		// WARNING: os.Setenv is process-global. Concurrent chat requests with
		// different skills will clobber each other's env vars. This is a known
		// limitation — a proper fix requires per-request env isolation in tool execution.
		envMu.Lock()
		for k, v := range sk.EnvVars {
			os.Setenv(k, v)
			skillEnvKeys = append(skillEnvKeys, k)
		}
		envMu.Unlock()
		}
	}
	// Unset skill env vars after the loop
	defer func() {
		envMu.Lock()
		for _, k := range skillEnvKeys {
			os.Unsetenv(k)
		}
		envMu.Unlock()
	}()

	if systemAppend != "" {
		systemContent += "\n\n" + systemAppend
	}
	systemMsg := types.Message{Role: "system", Content: systemContent}
	allMessages := append([]types.Message{systemMsg}, messages...)

	// Context compression: if messages are too long, compress old ones
	const maxContextMessages = 40
	const maxContextChars = 80000
	totalChars := 0
	for _, m := range allMessages {
		totalChars += len(m.Content)
	}
	if len(allMessages) > maxContextMessages || totalChars > maxContextChars {
		sseSend(c, flusher, "compact", "compressing context...", false)
		// Keep system message + last 20 messages
		keepCount := 20
		if len(allMessages)-1 < keepCount {
			keepCount = len(allMessages) - 1
		}
		oldMessages := allMessages[1 : len(allMessages)-keepCount]
		recentMessages := allMessages[len(allMessages)-keepCount:]
		// Summarize old messages into a brief context
		summary := "Previous conversation summary:\n"
		for _, m := range oldMessages {
			switch m.Role {
			case "user":
				summary += "- User: " + truncateStr(m.Content, 150) + "\n"
			case "assistant":
				if len(m.ToolCalls) > 0 {
					// Summarize tool calls instead of losing them
					for _, tc := range m.ToolCalls {
						summary += fmt.Sprintf("- Assistant called tool: %s\n", tc.Function.Name)
					}
				}
				if m.Content != "" {
					summary += "- Assistant: " + truncateStr(m.Content, 150) + "\n"
				}
			case "tool":
				summary += "- Tool result: " + truncateStr(m.Content, 100) + "\n"
			}
		}
		// Rebuild messages: system + summary + recent
		allMessages = append([]types.Message{systemMsg, {Role: "system", Content: "[Context compressed]\n" + summary}}, recentMessages...)
		sseSend(c, flusher, "compact_done", fmt.Sprintf("compressed %d messages", len(oldMessages)), false)
	}

	// Regex for detecting thinking tags
	thinkingRe := regexp.MustCompile(`(?s)<think>(.*?)</think>`)
	// Regex for detecting markdown images
	imageRe := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	// Regex for MiMo tool call delimiter tokens (leak into content field)
	toolDelimRe := regexp.MustCompile(`<｜tool[▁_](?:calls[▁_]begin|call[▁_]end|calls[▁_]end)｜>`)
	toolDelimPartialRe := regexp.MustCompile(`(?:<｜tool[▁_]|<｜tool[▁_]calls[▁_]|<｜tool[▁_]call[▁_])$`)

	hasContent := false
	for round := 0; round < maxRounds; round++ {
		// Check context timeout
		if ctx.Err() != nil {
			sseSend(c, flusher, "error", "request timeout", true)
			return
		}

		stream, err := provider.Stream(allMessages, schemas)
		if err != nil {
			sseSend(c, flusher, "error", err.Error(), true)
			return
		}

		var fullContent string
		var toolCalls []types.ToolCall

		for chunk := range stream.Chan {
			if chunk.Error != nil {
				sseSend(c, flusher, "error", chunk.Error.Error(), true)
				stream.Cancel()
				return
			}
			// Check context timeout on each chunk
			if ctx.Err() != nil {
				stream.Cancel()
				sseSend(c, flusher, "error", "request timeout", true)
				return
			}
			if chunk.Delta != "" {
				// Check for thinking content
				if strings.Contains(chunk.Delta, "<think>") || strings.Contains(chunk.Delta, "</think>") {
					thinkingMatches := thinkingRe.FindAllStringSubmatch(chunk.Delta, -1)
					for _, match := range thinkingMatches {
						if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
							thinkData, _ := json.Marshal(map[string]string{"agent": agent.Name, "content": strings.TrimSpace(match[1])})
							sseSend(c, flusher, "thinking", string(thinkData), false)
						}
					}
				}
				// Strip MiMo tool call delimiter tokens that leak into content
				cleanDelta := toolDelimRe.ReplaceAllString(chunk.Delta, "")
				// Also strip partial tokens at end of chunk (split across chunks)
				cleanDelta = toolDelimPartialRe.ReplaceAllString(cleanDelta, "")
				if cleanDelta == "" {
					continue
				}
				fullContent += cleanDelta
				sseSend(c, flusher, "delta", cleanDelta, false)
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.ToolCalls...)
			}
			if chunk.Done {
				break
			}
		}

		if len(toolCalls) == 0 {
			hasContent = true
			// Final cleanup: strip any remaining tool delimiter tokens
			fullContent = toolDelimRe.ReplaceAllString(fullContent, "")
			e.saveMsg(sessionID, types.Message{Role: "assistant", Content: fullContent, Name: agent.Name})
			log.Printf("[AGENT] Round %d: text response (%d chars)", round, len(fullContent))
			// Parse images from final content
			imageMatches := imageRe.FindAllStringSubmatch(fullContent, -1)
			for _, match := range imageMatches {
				alt := ""
				url := ""
				if len(match) > 1 {
					alt = match[1]
				}
				if len(match) > 2 {
					url = match[2]
				}
				if url != "" && !strings.HasPrefix(url, "data:") {
					imgData, _ := json.Marshal(map[string]string{"url": url, "alt": alt})
					sseSend(c, flusher, "image", string(imgData), false)
				}
			}
			return
		}

		assistantMsg := types.Message{Role: "assistant", Content: fullContent, ToolCalls: toolCalls, Name: agent.Name}
		e.saveMsg(sessionID, assistantMsg)
		allMessages = append(allMessages, assistantMsg)
		log.Printf("[AGENT] Round %d: %d tool calls requested", round, len(toolCalls))

		for _, tc := range toolCalls {
			// Send enhanced tool_call event with arguments
			var argsObj map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &argsObj)
			tcData, _ := json.Marshal(map[string]any{
				"agent": agent.Name,
				"name":  tc.Function.Name,
				"id":    tc.ID,
				"args":  argsObj,
			})
			sseSend(c, flusher, "tool_call", string(tcData), false)

		// Execute tool and measure duration
		start := time.Now()
		// NOTE: Tool goroutines may be orphaned on context timeout.
		// The goroutine will complete independently since Go doesn't support
		// goroutine cancellation. Tool handlers should ideally check ctx.Done()
		// for long-running operations, but this is not enforced at the interface level.
		type toolResultCh struct {
			result types.ToolResult
		}
		resultCh := make(chan toolResultCh, 1)
		go func() {
			resultCh <- toolResultCh{result: e.executeTool(tc, extraTools)}
		}()
		var result types.ToolResult
		select {
		case tr := <-resultCh:
			result = tr.result
		case <-ctx.Done():
			log.Printf("WARNING: tool %s goroutine may be orphaned due to context timeout", tc.Function.Name)
			result = types.ToolResult{ToolCallID: tc.ID, Content: "tool execution timeout"}
		}
		duration := time.Since(start).Milliseconds()

			log.Printf("[TOOL] %s completed in %dms, result=%d chars", tc.Function.Name, duration, len(result.Content))

			// Track for runtime skill engine
			e.runtimeSkillEngine.tracker.RecordCall(sessionID, TrackedCall{
				Tool:     tc.Function.Name,
				Args:     json.RawMessage(tc.Function.Arguments),
				Result:   result.Content,
				Duration: time.Duration(duration) * time.Millisecond,
				Time:     start,
			})

			// Send tool_result event
			trData, _ := json.Marshal(map[string]any{
				"agent":       agent.Name,
				"name":        tc.Function.Name,
				"id":          tc.ID,
				"content":     result.Content,
				"duration_ms": duration,
			})
			sseSend(c, flusher, "tool_result", string(trData), false)

			toolMsg := types.Message{Role: "tool", Content: result.Content, ToolCallID: tc.ID}
			e.saveMsg(sessionID, toolMsg)
			allMessages = append(allMessages, toolMsg)
		}
	}

	// After the loop ends, check if we have a final text response
	if !hasContent {
		// Make one final call without tools to get a summary
		resp, err := provider.Complete(allMessages, nil)
		if err == nil && resp.Message.Content != "" {
			summaryContent := resp.Message.Content
			sseSend(c, flusher, "delta", summaryContent, false)
			e.saveMsg(sessionID, types.Message{Role: "assistant", Content: summaryContent, Name: agent.Name})
			sseSend(c, flusher, "done", "", true)
		}
	}
}

// ════════════════════════════════════════════════
// Team handlers

func (e *ChatEngine) ListTeams(c *gin.Context) {
	user := c.MustGet("user").(*User)
	teams := e.teamStore.List(user.ID, user.Role == "admin")
	c.JSON(200, gin.H{"teams": teams})
}

func (e *ChatEngine) GetTeam(c *gin.Context) {
	team := e.teamStore.Get(c.Param("id"))
	if team == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{"team": team})
}

func (e *ChatEngine) CreateTeam(c *gin.Context) {
	userID := c.GetString("userID")
	var req struct {
		Name          string       `json:"name" binding:"required"`
		Description   string       `json:"description"`
		CoordinatorID string       `json:"coordinator_id" binding:"required"`
		Members       []TeamMember `json:"members"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	team := &Team{
		ID:            fmt.Sprintf("team-%d", time.Now().UnixNano()),
		Name:          req.Name,
		Description:   req.Description,
		CoordinatorID: req.CoordinatorID,
		Members:       req.Members,
		UserID:        userID,
		CreatedAt:     time.Now().Format(time.RFC3339),
	}
	e.teamStore.Create(team)
	c.JSON(200, gin.H{"team": team})
}

func (e *ChatEngine) UpdateTeam(c *gin.Context) {
	user := c.MustGet("user").(*User)
	userID := user.ID
	team := e.teamStore.Get(c.Param("id"))
	if team == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if team.UserID != userID && user.Role != "admin" {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	var req struct {
		Name          string       `json:"name"`
		Description   string       `json:"description"`
		CoordinatorID string       `json:"coordinator_id"`
		Members       []TeamMember `json:"members"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name != "" {
		team.Name = req.Name
	}
	if req.Description != "" {
		team.Description = req.Description
	}
	if req.CoordinatorID != "" {
		team.CoordinatorID = req.CoordinatorID
	}
	if req.Members != nil {
		team.Members = req.Members
	}
	e.teamStore.Update(team)
	c.JSON(200, gin.H{"team": team})
}

func (e *ChatEngine) DeleteTeam(c *gin.Context) {
	user := c.MustGet("user").(*User)
	userID := user.ID
	team := e.teamStore.Get(c.Param("id"))
	if team == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if team.UserID != userID && user.Role != "admin" {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	e.teamStore.Delete(c.Param("id"))
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) buildTeamPrompt(team *Team) string {
	var sb strings.Builder
	sb.WriteString("你是一个团队协调者。你可以将子任务委派给以下团队成员：\n\n")
	for _, m := range team.Members {
		agent := e.agentStore.Get(m.AgentID)
		desc := ""
		if agent != nil {
			desc = agent.Description
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", m.Name, m.Role, desc))
	}
	sb.WriteString("\n使用 delegate_task 工具委派任务给成员。\n")
	sb.WriteString("每个成员会独立执行任务并返回结果。\n")
	sb.WriteString("你需要：\n")
	sb.WriteString("1. 分析用户的请求，判断需要哪些成员参与\n")
	sb.WriteString("2. 将任务拆解为子任务，分配给合适的成员\n")
	sb.WriteString("3. 收集所有成员的结果\n")
	sb.WriteString("4. 汇总并整合为完整的回答\n\n")
	sb.WriteString("注意：每个子任务应该独立、明确、可执行。如果任务有依赖关系，按顺序委派。")
	return sb.String()
}

func (e *ChatEngine) createDelegateHandler(team *Team, c *gin.Context, flusher http.Flusher, userID string) ToolHandler {
	return ToolHandler{
		Schema: types.ToolSchema{
			Name:        "delegate_task",
			Description: fmt.Sprintf("将子任务委派给团队成员执行。可用成员: %s", team.MemberNames()),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"member_name": map[string]any{
						"type":        "string",
						"description": "团队成员名称",
					},
					"task": map[string]any{
						"type":        "string",
						"description": "要委派的任务描述",
					},
				},
				"required": []string{"member_name", "task"},
			},
		},
		Handler: func(tc types.ToolCall) types.ToolResult {
			var params struct {
				MemberName string `json:"member_name"`
				Task       string `json:"task"`
			}
			json.Unmarshal([]byte(tc.Function.Arguments), &params)

			var member *TeamMember
			for i, m := range team.Members {
				if m.Name == params.MemberName {
					member = &team.Members[i]
					break
				}
			}
			if member == nil {
				return types.ToolResult{ToolCallID: tc.ID, Content: fmt.Sprintf("找不到团队成员: %s", params.MemberName)}
			}

			agent := e.agentStore.Get(member.AgentID)
			if agent == nil {
				return types.ToolResult{ToolCallID: tc.ID, Content: fmt.Sprintf("成员配置不存在: %s", member.AgentID)}
			}

			// Send progress event
			pd, _ := json.Marshal(map[string]string{"name": member.Name, "role": member.Role, "status": "start", "task": params.Task})
			sseSend(c, flusher, "team_progress", string(pd), false)

			result := e.runSubAgentSync(c.Request.Context(), agent, params.Task, userID)

			pd, _ = json.Marshal(map[string]string{"name": member.Name, "role": member.Role, "status": "done"})
			sseSend(c, flusher, "team_progress", string(pd), false)

			return types.ToolResult{ToolCallID: tc.ID, Content: result}
		},
	}
}

func (e *ChatEngine) runSubAgentSync(ctx context.Context, agent *AgentConfig, task string, userID string) string {
	provider := e.getProvider(agent.Model, userID)
	schemas := e.getSchemas(agent.Tools)
	messages := []types.Message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: task},
	}

	for round := 0; round < 5; round++ {
		stream, err := provider.Stream(messages, schemas)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		var fullContent string
		var toolCalls []types.ToolCall
		for chunk := range stream.Chan {
			if chunk.Error != nil {
				stream.Cancel()
				return fmt.Sprintf("Error: %v", chunk.Error)
			}
			fullContent += chunk.Delta
			if len(chunk.ToolCalls) > 0 {
				toolCalls = append(toolCalls, chunk.ToolCalls...)
			}
			if chunk.Done {
				break
			}
		}

		// Strip MiMo tool call delimiter tokens
		toolDelimRe := regexp.MustCompile(`<｜tool[▁_](?:calls[▁_]begin|call[▁_]end|calls[▁_]end)｜>`)
		fullContent = toolDelimRe.ReplaceAllString(fullContent, "")

		if len(toolCalls) == 0 {
			return fullContent
		}

		assistantMsg := types.Message{Role: "assistant", Content: fullContent, ToolCalls: toolCalls}
		messages = append(messages, assistantMsg)

		for _, tc := range toolCalls {
			result := e.executeTool(tc, nil)
			toolMsg := types.Message{Role: "tool", Content: result.Content, ToolCallID: tc.ID}
			messages = append(messages, toolMsg)
		}
	}
	return "Max rounds exceeded"
}

// ════════════════════════════════════════════════
// Helpers

func generateTitle(msgs []ChatMessage) string {
	for _, m := range msgs {
		if m.Role == "user" && m.Content != "" {
			if len(m.Content) > 40 {
				return m.Content[:40] + "..."
			}
			return m.Content
		}
	}
	return "New Chat"
}

func (e *ChatEngine) generateAITitle(sessionID string, firstMsg string, model string, userID string, c *gin.Context, flusher http.Flusher, wg *sync.WaitGroup) {
	defer wg.Done()
	messages := []types.Message{
		{Role: "user", Content: fmt.Sprintf("Generate a very short title (max 20 chars) for a conversation that starts with: %s. Reply with ONLY the title, no quotes.", firstMsg)},
	}
	provider := e.getProvider(model, userID)
	resp, err := provider.Complete(messages, nil)
	if err != nil {
		return
	}
	title := strings.TrimSpace(resp.Message.Content)
	if title == "" {
		return
	}
	if len(title) > 50 {
		title = title[:50]
	}
	e.sessionStore.UpdateTitle(sessionID, title)
	sseSend(c, flusher, "title_update", title, false)
}

func (e *ChatEngine) saveMsg(sessionID string, msg types.Message) {
	stored := &store.StoredMessage{
		ID: fmt.Sprintf("msg-%d", time.Now().UnixNano()), SessionID: sessionID,
		Role: msg.Role, Content: msg.Content, Name: msg.Name,
	}
	if len(msg.ToolCalls) > 0 {
		data, _ := json.Marshal(msg.ToolCalls)
		stored.ToolCalls = data
	}
	stored.ToolCallID = msg.ToolCallID
	e.store.AppendMessage(sessionID, stored)
}

func (e *ChatEngine) loadImage(img ImageAttach) (string, string, error) {
	if img.Filename != "" {
		fullPath := filepath.Join(uploadDir, img.Filename)
		if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(uploadDir)) {
			return "", "", fmt.Errorf("path traversal detected")
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return "", "", err
		}
		ext := strings.ToLower(filepath.Ext(img.Filename))
		mime := "image/png"
		switch ext {
		case ".jpg", ".jpeg":
			mime = "image/jpeg"
		case ".gif":
			mime = "image/gif"
		case ".webp":
			mime = "image/webp"
		}
		return base64.StdEncoding.EncodeToString(data), mime, nil
	}
	return "", "", fmt.Errorf("no image source")
}

func sseSend(c *gin.Context, flusher http.Flusher, event, data string, done bool) {
	payload, _ := json.Marshal(map[string]any{"event": event, "data": data, "done": done})
	fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
	flusher.Flush()
}

func (e *ChatEngine) HandleChatCancel(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SessionID == "" {
		c.JSON(400, gin.H{"error": "session_id required"})
		return
	}
	e.activeCancelsMu.Lock()
	cancel, ok := e.activeCancels[req.SessionID]
	e.activeCancelsMu.Unlock()
	if !ok {
		c.JSON(200, gin.H{"ok": true, "msg": "no active session (already finished?)"})
		return
	}
	cancel()
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) HandleModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"models":  []string{e.defaultProvider.Model()},
		"current": e.defaultProvider.Model(),
		"engine":  "crux-go",
	})
}

func handleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "no file"})
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true}
	if !allowed[ext] {
		c.JSON(400, gin.H{"error": "only images"})
		return
	}
	if file.Size > 10*1024*1024 {
		c.JSON(400, gin.H{"error": "too large"})
		return
	}
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	if err := c.SaveUploadedFile(file, filepath.Join(uploadDir, filename)); err != nil {
		c.JSON(500, gin.H{"error": "save failed"})
		return
	}
	c.JSON(200, gin.H{"url": "/uploads/" + filename, "filename": filename, "size": file.Size})
}

// ════════════════════════════════════════════════
// Model Provider handlers

// maskAPIKey returns a masked version of the API key showing only last 4 chars
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

func (e *ChatEngine) ListProviders(c *gin.Context) {
	userID := c.GetString("userID")
	providers := e.providerStore.ListByUser(userID)
	// Mask API keys in response
	result := make([]gin.H, 0, len(providers))
	for _, p := range providers {
		result = append(result, gin.H{
			"id":         p.ID,
			"user_id":    p.UserID,
			"name":       p.Name,
			"base_url":   p.BaseURL,
			"api_key":    maskAPIKey(p.APIKey),
			"models":     p.Models,
			"default":    p.Default,
			"builtin":    p.Builtin,
			"created_at": p.CreatedAt,
		})
	}
	c.JSON(200, gin.H{"providers": result})
}

func (e *ChatEngine) CreateProvider(c *gin.Context) {
	userID := c.GetString("userID")
	var p ModelProvider
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	p.UserID = userID
	e.providerStore.Create(&p)
	c.JSON(200, p)
}

func (e *ChatEngine) UpdateProvider(c *gin.Context) {
	user := c.MustGet("user").(*User)
	userID := user.ID
	id := c.Param("id")
	existing := e.providerStore.Get(id)
	if existing == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if existing.Builtin && user.Role != "admin" {
		c.JSON(403, gin.H{"error": "cannot modify builtin provider"})
		return
	}
	if existing.UserID != userID && !existing.Builtin {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	var p ModelProvider
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	e.providerStore.Update(id, &p)
	c.JSON(200, p)
}

func (e *ChatEngine) DeleteProvider(c *gin.Context) {
	userID := c.GetString("userID")
	if err := e.providerStore.Delete(c.Param("id"), userID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) ListAvailableModels(c *gin.Context) {
	userID := c.GetString("userID")
	providers := e.providerStore.ListByUser(userID)
	type ModelEntry struct {
		ProviderID   string `json:"provider_id"`
		ProviderName string `json:"provider_name"`
		Model        string `json:"model"`
		FullID       string `json:"full_id"` // providerID/model
	}
	var models []ModelEntry
	for _, p := range providers {
		for _, m := range p.Models {
			models = append(models, ModelEntry{
				ProviderID:   p.ID,
				ProviderName: p.Name,
				Model:        m,
				FullID:       p.ID + "/" + m,
			})
		}
	}
	c.JSON(200, gin.H{"models": models})
}

// ════════════════════════════════════════════════
// Skill handlers

func (e *ChatEngine) ListSkills(c *gin.Context) {
	userID := c.GetString("userID")
	c.JSON(200, gin.H{"skills": e.skillStore.ListByUser(userID)})
}

func (e *ChatEngine) CreateSkill(c *gin.Context) {
	userID := c.GetString("userID")
	var sk UserSkill
	if err := c.ShouldBindJSON(&sk); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	sk.UserID = userID
	e.skillStore.Create(&sk)
	c.JSON(200, sk)
}

func (e *ChatEngine) UpdateSkill(c *gin.Context) {
	userID := c.GetString("userID")
	id := c.Param("id")
	existing := e.skillStore.Get(id)
	if existing == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// Admin can update builtins, users can only update their own
	user := c.MustGet("user").(*User)
	if existing.UserID != userID && !existing.Builtin {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}
	if existing.Builtin && user.Role != "admin" {
		c.JSON(403, gin.H{"error": "admin only"})
		return
	}
	var sk UserSkill
	if err := c.ShouldBindJSON(&sk); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	e.skillStore.Update(id, &sk)
	c.JSON(200, sk)
}

func (e *ChatEngine) DeleteSkill(c *gin.Context) {
	userID := c.GetString("userID")
	if err := e.skillStore.Delete(c.Param("id"), userID); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) CreateBuiltinSkill(c *gin.Context) {
	user := c.MustGet("user").(*User)
	if user.Role != "admin" {
		c.JSON(403, gin.H{"error": "admin only"})
		return
	}
	var sk UserSkill
	if err := c.ShouldBindJSON(&sk); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	sk.Builtin = true
	e.skillStore.Create(&sk)
	c.JSON(200, sk)
}

func (e *ChatEngine) ListBuiltinSkills(c *gin.Context) {
	user := c.MustGet("user").(*User)
	if user.Role != "admin" {
		c.JSON(403, gin.H{"error": "admin only"})
		return
	}
	c.JSON(200, gin.H{"skills": e.skillStore.ListBuiltin()})
}

// Curator handlers

func (e *ChatEngine) CuratorStatus(c *gin.Context) {
	c.JSON(200, e.skillStore.CuratorStatus())
}

func (e *ChatEngine) RunCurator(c *gin.Context) {
	var body struct {
		StaleDays int `json:"stale_days"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.StaleDays = 30
	}
	if body.StaleDays <= 0 {
		body.StaleDays = 30
	}
	count := e.skillStore.RunCurator(body.StaleDays)
	c.JSON(200, gin.H{"archived": count, "stale_days": body.StaleDays})
}

func (e *ChatEngine) PinSkill(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Pinned = true
	}
	if err := e.skillStore.Pin(id, body.Pinned); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "pinned": body.Pinned})
}

func (e *ChatEngine) ArchiveSkill(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Archived bool `json:"archived"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Archived = true
	}
	if err := e.skillStore.Archive(id, body.Archived); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "archived": body.Archived})
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
