package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ════════════════════════════════════════════════
// Feishu Adapter (webhook-based)
// ════════════════════════════════════════════════

type FeishuAdapter struct {
	appID             string
	appSecret         string
	verificationToken string
	manager           *GatewayManager
	ctx               context.Context
	cancel            context.CancelFunc
	running           bool
	mu                sync.Mutex
	accessToken       string
	tokenExpiry       time.Time
	tokenMu           sync.Mutex
	client            *http.Client
}

func NewFeishuAdapter(appID, appSecret, verificationToken string, manager *GatewayManager) *FeishuAdapter {
	return &FeishuAdapter{
		appID:             appID,
		appSecret:         appSecret,
		verificationToken: verificationToken,
		manager:           manager,
		client:            &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *FeishuAdapter) Name() string { return "feishu" }

func (a *FeishuAdapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *FeishuAdapter) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.running = true
	log.Printf("feishu: started (webhook mode)")
	return nil
}

func (a *FeishuAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return nil
	}
	a.cancel()
	a.running = false
	log.Printf("feishu: stopped")
	return nil
}

func (a *FeishuAdapter) getAccessToken() (string, error) {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()

	if a.accessToken != "" && time.Now().Before(a.tokenExpiry) {
		return a.accessToken, nil
	}

	url := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	body := map[string]string{
		"app_id":     a.appID,
		"app_secret": a.appSecret,
	}
	data, _ := json.Marshal(body)
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Code != 0 {
		return "", fmt.Errorf("feishu token error: %s", result.Msg)
	}

	a.accessToken = result.TenantAccessToken
	a.tokenExpiry = time.Now().Add(time.Duration(result.Expire-60) * time.Second)
	return a.accessToken, nil
}

// HandleWebhook handles Feishu event subscription POST
func (a *FeishuAdapter) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(200, gin.H{"ok": true})
		return
	}

	var event struct {
		Type  string `json:"type"`
		Token string `json:"token"`
		Event struct {
			Message struct {
				ChatID      string `json:"chat_id"`
				Content     string `json:"content"`
				MessageType string `json:"message_type"`
				Sender      struct {
					SenderID struct {
						UserID string `json:"user_id"`
					} `json:"sender_id"`
					SenderType string `json:"sender_type"`
				} `json:"sender"`
			} `json:"message"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		// Try as raw map for URL verification
		var raw map[string]any
		if json.Unmarshal(body, &raw) == nil {
			if t, ok := raw["type"].(string); ok && t == "url_verification" {
				if ch, ok := raw["challenge"].(string); ok {
					c.JSON(200, gin.H{"challenge": ch})
					return
				}
			}
		}
		c.JSON(200, gin.H{"ok": true})
		return
	}

	// URL verification
	if event.Type == "url_verification" {
		c.JSON(200, gin.H{"challenge": event.Token})
		return
	}

	// ── Authentication gate ──────────────────────────────────────────────
	// This endpoint is public and reaches the agent loop, which may hold shell
	// tools. It must fail closed.
	//
	// The previous check only rejected when a token was present AND mismatched,
	// so omitting the "token" field entirely bypassed verification completely.
	// Verify before dispatching any event.
	if a.verificationToken == "" {
		log.Printf("SECURITY: rejecting Feishu webhook — no verification_token configured. " +
			"Set it in the gateway config, otherwise this public endpoint can drive the agent unauthenticated.")
		c.JSON(503, gin.H{"error": "webhook verification not configured"})
		return
	}
	if event.Token == "" {
		c.JSON(403, gin.H{"error": "missing verification token"})
		return
	}
	if event.Token != a.verificationToken {
		c.JSON(403, gin.H{"error": "invalid verification token"})
		return
	}

	// Message event
	if event.Type == "event_callback" && event.Event.Message.ChatID != "" {
		// Parse content JSON (e.g. {"text":"hello"})
		content := event.Event.Message.Content
		var contentMap map[string]string
		json.Unmarshal([]byte(content), &contentMap)
		text := content
		if t, ok := contentMap["text"]; ok {
			text = t
		}

		msg := GatewayMessage{
			Platform:  "feishu",
			ChatID:    event.Event.Message.ChatID,
			UserID:    event.Event.Message.Sender.SenderID.UserID,
			UserName:  event.Event.Message.Sender.SenderID.UserID,
			Content:   text,
			MsgType:   "text",
			Timestamp: time.Now().Unix(),
		}

		if event.Event.Message.MessageType == "image" {
			msg.MsgType = "image"
		}

		go a.manager.ProcessMessage(msg)
	}

	c.JSON(200, gin.H{"ok": true})
}

func (a *FeishuAdapter) SendMessage(chatID, text string) error {
	token, err := a.getAccessToken()
	if err != nil {
		return fmt.Errorf("feishu get token: %w", err)
	}

	url := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"
	content, _ := json.Marshal(map[string]string{"text": text})
	body := map[string]any{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("feishu sendMessage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("feishu sendMessage [%d]: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (a *FeishuAdapter) SendImage(chatID, imageURL string) error {
	// Feishu requires uploading image first, then sending as image message
	// For simplicity, send as text with URL
	return a.SendMessage(chatID, "[Image] "+imageURL)
}
