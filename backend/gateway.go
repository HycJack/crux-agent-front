package main

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hermes-go/core/store"
	"github.com/hermes-go/core/types"
)

// ════════════════════════════════════════════════
// Gateway Message (common format)
// ════════════════════════════════════════════════

type GatewayMessage struct {
	Platform  string   `json:"platform"`
	ChatID    string   `json:"chat_id"`
	UserID    string   `json:"user_id"`
	UserName  string   `json:"user_name"`
	Content   string   `json:"content"`
	Images    []string `json:"images,omitempty"`
	MsgType   string   `json:"msg_type"` // "text", "image"
	Timestamp int64    `json:"timestamp"`
}

// ════════════════════════════════════════════════
// Platform Adapter Interface
// ════════════════════════════════════════════════

type PlatformAdapter interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	SendMessage(chatID, text string) error
	SendImage(chatID, imageURL string) error
	IsRunning() bool
}

// ════════════════════════════════════════════════
// Gateway Session Store
// ════════════════════════════════════════════════

type GatewaySession struct {
	Platform     string `json:"platform"`
	PlatformChat string `json:"platform_chat"`
	SessionID    string `json:"session_id"`
	UserID       string `json:"user_id"`
	AgentID      string `json:"agent_id"`
	CreatedAt    string `json:"created_at"`
}

type GatewaySessionStore struct {
	path     string
	mu       sync.RWMutex
	sessions map[string]*GatewaySession // key: "platform:chatID"
}

func NewGatewaySessionStore(path string) *GatewaySessionStore {
	s := &GatewaySessionStore{
		path:     path,
		sessions: make(map[string]*GatewaySession),
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	s.load()
	return s
}

func (s *GatewaySessionStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var items []*GatewaySession
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("GatewaySessionStore load error: %v", err)
		return
	}
	for _, item := range items {
		key := item.Platform + ":" + item.PlatformChat
		s.sessions[key] = item
	}
}

func (s *GatewaySessionStore) save() {
	list := make([]*GatewaySession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, sess)
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("GatewaySessionStore save error: %v", err)
	}
}

func (s *GatewaySessionStore) Get(platform, chatID string) *GatewaySession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := platform + ":" + chatID
	return s.sessions[key]
}

func (s *GatewaySessionStore) GetByKey(key string) *GatewaySession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[key]
}

func (s *GatewaySessionStore) Set(sess *GatewaySession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sess.Platform + ":" + sess.PlatformChat
	s.sessions[key] = sess
	s.save()
}

// GetByPlatform returns all sessions for a given platform.
func (s *GatewaySessionStore) GetByPlatform(platform string) []*GatewaySession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*GatewaySession
	for _, sess := range s.sessions {
		if sess.Platform == platform {
			result = append(result, sess)
		}
	}
	return result
}

// DeleteByPlatform removes all sessions for a given platform.
func (s *GatewaySessionStore) DeleteByPlatform(platform string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for key, sess := range s.sessions {
		if sess.Platform == platform {
			delete(s.sessions, key)
			count++
		}
	}
	if count > 0 {
		s.save()
	}
	return count
}

// ════════════════════════════════════════════════
// Gateway Manager
// ════════════════════════════════════════════════

type GatewayManager struct {
	engine   *ChatEngine
	config   *GatewayConfigStore
	sessions *GatewaySessionStore
	adapters map[string]PlatformAdapter
	running  map[string]context.CancelFunc
	mu       sync.Mutex
}

func NewGatewayManager(engine *ChatEngine, config *GatewayConfigStore, sessionPath string) *GatewayManager {
	return &GatewayManager{
		engine:   engine,
		config:   config,
		sessions: NewGatewaySessionStore(sessionPath),
		adapters: make(map[string]PlatformAdapter),
		running:  make(map[string]context.CancelFunc),
	}
}

// Start reads config and starts all enabled adapters.
func (gm *GatewayManager) Start() {
	configs := gm.config.List()
	for platform, cfg := range configs {
		if cfg.Enabled {
			if err := gm.startPlatform(platform, cfg); err != nil {
				log.Printf("gateway: failed to start %s: %v", platform, err)
			}
		}
	}
}

func (gm *GatewayManager) startPlatform(platform string, cfg *PlatformConfig) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Stop existing if running
	if cancel, ok := gm.running[platform]; ok {
		cancel()
		delete(gm.running, platform)
	}

	adapter := gm.createAdapter(platform, cfg)
	if adapter == nil {
		return fmt.Errorf("unknown platform: %s", platform)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := adapter.Start(ctx); err != nil {
		cancel()
		return err
	}

	gm.adapters[platform] = adapter
	gm.running[platform] = cancel
	return nil
}

func (gm *GatewayManager) createAdapter(platform string, cfg *PlatformConfig) PlatformAdapter {
	switch platform {
	case "telegram":
		token := cfg.Settings["bot_token"]
		if token == "" {
			return nil
		}
		return NewTelegramAdapter(token, gm)
	case "feishu":
		appID := cfg.Settings["app_id"]
		appSecret := cfg.Settings["app_secret"]
		verificationToken := cfg.Settings["verification_token"]
		if appID == "" || appSecret == "" {
			return nil
		}
		return NewFeishuAdapter(appID, appSecret, verificationToken, gm)
	case "wechat":
		botToken := cfg.Settings["bot_token"]
		sessionKey := cfg.Settings["session_key"]
		if botToken == "" {
			return nil
		}
		return NewWeChatAdapter(botToken, sessionKey, gm)
	default:
		return nil
	}
}

// StartPlatform starts a single platform by name.
func (gm *GatewayManager) StartPlatform(platform string) error {
	cfg := gm.config.Get(platform)
	if cfg == nil {
		return fmt.Errorf("platform %s not configured", platform)
	}
	cfg.Enabled = true
	gm.config.Set(platform, cfg)
	return gm.startPlatform(platform, cfg)
}

// StopPlatform stops a single platform by name.
func (gm *GatewayManager) StopPlatform(platform string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if cancel, ok := gm.running[platform]; ok {
		cancel()
		delete(gm.running, platform)
	}
	if adapter, ok := gm.adapters[platform]; ok {
		adapter.Stop()
		delete(gm.adapters, platform)
	}
	return nil
}

// RestartPlatform stops then starts a platform.
func (gm *GatewayManager) RestartPlatform(platform string) error {
	gm.StopPlatform(platform)
	return gm.StartPlatform(platform)
}

// Status returns the running status of all configured platforms.
func (gm *GatewayManager) Status() map[string]map[string]any {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	configs := gm.config.List()
	result := make(map[string]map[string]any)
	for platform := range configs {
		running := false
		if adapter, ok := gm.adapters[platform]; ok {
			running = adapter.IsRunning()
		}
		result[platform] = map[string]any{
			"running": running,
			"enabled": configs[platform].Enabled,
		}
	}
	return result
}

// GetAdapter returns the adapter for a platform, or nil.
func (gm *GatewayManager) GetAdapter(platform string) PlatformAdapter {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	return gm.adapters[platform]
}

// getOrCreateUser finds or creates an internal user for a platform user
func (gm *GatewayManager) getOrCreateUser(platform, platformUserID, userName string) string {
	key := platform + ":" + platformUserID
	if sess := gm.sessions.GetByKey(key); sess != nil {
		return sess.UserID
	}
	username := platform + "_" + platformUserID
	// These accounts are reachable only through the messaging gateway. Give each
	// one a random 256-bit secret so the account can never be claimed over
	// POST /api/auth/login, and never let it take the first-user admin slot.
	randomSecret := make([]byte, 32)
	if _, err := crand.Read(randomSecret); err != nil {
		log.Printf("gateway: failed to generate secret for %s: %v", username, err)
		return ""
	}
	user, _ := gm.engine.userStore.CreateInternal(username, hex.EncodeToString(randomSecret), userName)
	if user == nil {
		// User might already exist with that username, try to find by name
		existing := gm.engine.userStore.GetByUsername(username)
		if existing != nil {
			return existing.ID
		}
		return ""
	}
	return user.ID
}

// ProcessMessage handles a normalized gateway message: find/create session, run agent, respond
func (gm *GatewayManager) ProcessMessage(msg GatewayMessage) {
	adapter := gm.GetAdapter(msg.Platform)
	if adapter == nil {
		log.Printf("gateway: unknown platform %s", msg.Platform)
		return
	}

	// Get or create session
	gwSession := gm.sessions.Get(msg.Platform, msg.ChatID)
	if gwSession == nil {
		userID := gm.getOrCreateUser(msg.Platform, msg.UserID, msg.UserName)
		if userID == "" {
			log.Printf("gateway: failed to create user for %s:%s", msg.Platform, msg.UserID)
			return
		}
		sessionID := fmt.Sprintf("gw-sess-%d", time.Now().UnixNano())
		gwSession = &GatewaySession{
			Platform:     msg.Platform,
			PlatformChat: msg.ChatID,
			SessionID:    sessionID,
			UserID:       userID,
			AgentID:      "default",
			CreatedAt:    time.Now().Format(time.RFC3339),
		}
		gm.sessions.Set(gwSession)

		// Create internal session
		gm.engine.sessionStore.Create(&SessionMeta{
			ID:       sessionID,
			UserID:   userID,
			Title:    fmt.Sprintf("Gateway: %s/%s", msg.Platform, msg.ChatID),
			AgentIDs: []string{"default"},
		})
	}

	// Build user message
	userContent := msg.Content
	if msg.MsgType == "image" && len(msg.Images) > 0 {
		userContent = "[Image: " + strings.Join(msg.Images, ", ") + "]"
		if msg.Content != "" {
			userContent = msg.Content + " " + userContent
		}
	}

	// Save user message to store
	userMsg := types.Message{Role: "user", Content: userContent}
	gm.engine.saveMsg(gwSession.SessionID, userMsg)

	// Load history
	history, _ := gm.engine.store.GetMessages(gwSession.SessionID, store.MessageOpts{Limit: 500})
	var messages []types.Message
	for _, m := range history {
		histMsg := types.Message{Role: m.Role, Content: m.Content, Name: m.Name}
		if len(m.ToolCalls) > 0 {
			json.Unmarshal(m.ToolCalls, &histMsg.ToolCalls)
		}
		histMsg.ToolCallID = m.ToolCallID
		messages = append(messages, histMsg)
	}

	// Run agent sync
	agentID := gwSession.AgentID
	if agentID == "" {
		agentID = "default"
	}
	response, err := gm.engine.RunAgentSync(agentID, messages, gwSession.UserID)
	if err != nil {
		log.Printf("gateway: agent error: %v", err)
		adapter.SendMessage(msg.ChatID, "Sorry, an error occurred: "+err.Error())
		return
	}

	// Save assistant response
	gm.engine.saveMsg(gwSession.SessionID, types.Message{Role: "assistant", Content: response})
	gm.engine.sessionStore.Touch(gwSession.SessionID)

	// Send response back via platform
	if err := adapter.SendMessage(msg.ChatID, response); err != nil {
		log.Printf("gateway: send error: %v", err)
	}
}

// ════════════════════════════════════════════════
// RunAgentSync — non-streaming agent loop
// ════════════════════════════════════════════════

func (e *ChatEngine) RunAgentSync(agentID string, messages []types.Message, userID string) (string, error) {
	agent := e.agentStore.Get(agentID)
	if agent == nil {
		return "", fmt.Errorf("agent not found: %s", agentID)
	}
	provider := e.getProvider(agent.Model, userID)
	schemas := e.getSchemas(agent.Tools)
	maxRounds := agent.MaxRounds
	if maxRounds == 0 {
		maxRounds = 10
	}

	systemMsg := types.Message{Role: "system", Content: agent.SystemPrompt}
	allMessages := append([]types.Message{systemMsg}, messages...)

	for round := 0; round < maxRounds; round++ {
		resp, err := provider.Complete(allMessages, schemas)
		if err != nil {
			return "", err
		}

		if len(resp.Message.ToolCalls) == 0 {
			return resp.Message.Content, nil
		}

		assistantMsg := types.Message{Role: "assistant", Content: resp.Message.Content, ToolCalls: resp.Message.ToolCalls}
		allMessages = append(allMessages, assistantMsg)

		for _, tc := range resp.Message.ToolCalls {
			result := e.executeTool(tc)
			toolMsg := types.Message{Role: "tool", Content: result.Content, ToolCallID: tc.ID}
			allMessages = append(allMessages, toolMsg)
		}
	}
	return "", fmt.Errorf("max rounds (%d) exceeded", maxRounds)
}

// ════════════════════════════════════════════════
// Gateway HTTP Handlers (for main.go route registration)
// ════════════════════════════════════════════════

// ListConfig returns all platform configs.
func (gm *GatewayManager) ListConfig(c *gin.Context) {
	configs := gm.config.List()
	c.JSON(200, gin.H{"configs": configs})
}

// GetConfig returns the config for a specific platform.
func (gm *GatewayManager) GetConfig(c *gin.Context) {
	platform := c.Param("platform")
	cfg := gm.config.Get(platform)
	if cfg == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{"platform": platform, "config": cfg})
}

// SetConfig creates or updates the config for a platform.
func (gm *GatewayManager) SetConfig(c *gin.Context) {
	platform := c.Param("platform")
	var cfg PlatformConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	gm.config.Set(platform, &cfg)
	c.JSON(200, gin.H{"ok": true, "platform": platform})
}

// DeleteConfig removes a platform config and stops it if running.
func (gm *GatewayManager) DeleteConfig(c *gin.Context) {
	platform := c.Param("platform")
	gm.StopPlatform(platform)
	if !gm.config.Delete(platform) {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// HandleStartPlatform starts a platform.
func (gm *GatewayManager) HandleStartPlatform(c *gin.Context) {
	platform := c.Param("platform")
	if err := gm.StartPlatform(platform); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "platform": platform})
}

// HandleStopPlatform stops a platform.
func (gm *GatewayManager) HandleStopPlatform(c *gin.Context) {
	platform := c.Param("platform")
	if err := gm.StopPlatform(platform); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "platform": platform})
}

// HandleStatus returns the status of all platforms.
func (gm *GatewayManager) HandleStatus(c *gin.Context) {
	status := gm.Status()
	c.JSON(200, gin.H{"status": status})
}

// HandleFeishuWebhook forwards Feishu webhook to the adapter.
func (gm *GatewayManager) HandleFeishuWebhook(c *gin.Context) {
	adapter := gm.GetAdapter("feishu")
	if adapter == nil {
		c.JSON(200, gin.H{"ok": true})
		return
	}
	feishuAdapter, ok := adapter.(*FeishuAdapter)
	if !ok {
		c.JSON(200, gin.H{"ok": true})
		return
	}
	feishuAdapter.HandleWebhook(c)
}

// HandleTelegramWebhook forwards Telegram webhook to the adapter.
func (gm *GatewayManager) HandleTelegramWebhook(c *gin.Context) {
	adapter := gm.GetAdapter("telegram")
	if adapter == nil {
		c.JSON(200, gin.H{"ok": true})
		return
	}
	tgAdapter, ok := adapter.(*TelegramAdapter)
	if !ok {
		c.JSON(200, gin.H{"ok": true})
		return
	}
	tgAdapter.HandleWebhook(c)
}

// GetWeChatQR returns a QR code URL for WeChat login.
func (gm *GatewayManager) GetWeChatQR(c *gin.Context) {
	adapter := gm.GetAdapter("wechat")
	if adapter == nil {
		c.JSON(400, gin.H{"error": "wechat adapter not running"})
		return
	}
	wxAdapter, ok := adapter.(*WeChatAdapter)
	if !ok {
		c.JSON(400, gin.H{"error": "invalid adapter type"})
		return
	}
	qrURL, err := wxAdapter.GetQRCode()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"qr_url": qrURL})
}

// GetWeChatQRStatus checks the QR code scan status.
func (gm *GatewayManager) GetWeChatQRStatus(c *gin.Context) {
	adapter := gm.GetAdapter("wechat")
	if adapter == nil {
		c.JSON(400, gin.H{"error": "wechat adapter not running"})
		return
	}
	wxAdapter, ok := adapter.(*WeChatAdapter)
	if !ok {
		c.JSON(400, gin.H{"error": "invalid adapter type"})
		return
	}
	status, sessionKey, err := wxAdapter.CheckQRStatus()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": status, "session_key": sessionKey})
}

// GetGatewaySessions returns all gateway sessions.
func (gm *GatewayManager) GetGatewaySessions(c *gin.Context) {
	platform := c.Query("platform")
	if platform != "" {
		sessions := gm.sessions.GetByPlatform(platform)
		c.JSON(200, gin.H{"sessions": sessions})
		return
	}
	// Return all
	var all []*GatewaySession
	gm.sessions.mu.RLock()
	for _, sess := range gm.sessions.sessions {
		all = append(all, sess)
	}
	gm.sessions.mu.RUnlock()
	c.JSON(200, gin.H{"sessions": all})
}
