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
// Telegram Adapter (polling + webhook)
// ════════════════════════════════════════════════

const telegramAPIBase = "https://api.telegram.org"

type TelegramAdapter struct {
	token      string
	manager    *GatewayManager
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
	mu         sync.Mutex
	webhookURL string // if set, use webhook mode instead of polling
	client     *http.Client
}

func NewTelegramAdapter(token string, manager *GatewayManager) *TelegramAdapter {
	return &TelegramAdapter{
		token:   token,
		manager: manager,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (a *TelegramAdapter) Name() string { return "telegram" }

func (a *TelegramAdapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *TelegramAdapter) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.running = true
	go a.pollLoop()
	log.Printf("telegram: polling started")
	return nil
}

func (a *TelegramAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return nil
	}
	a.cancel()
	a.running = false
	log.Printf("telegram: stopped")
	return nil
}

func (a *TelegramAdapter) pollLoop() {
	offset := 0
	backoff := 1 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=30", telegramAPIBase, a.token, offset)
		resp, err := a.client.Get(url)
		if err != nil {
			log.Printf("telegram: poll error: %v", err)
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = backoff * 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("telegram: read body error: %v", err)
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		backoff = 1 * time.Second // reset on success

		var result struct {
			OK     bool `json:"ok"`
			Result []struct {
				UpdateID int64 `json:"update_id"`
				Message  struct {
					Chat struct {
						ID    int64  `json:"id"`
						Title string `json:"title"`
					} `json:"chat"`
					From struct {
						ID        int64  `json:"id"`
						FirstName string `json:"first_name"`
						LastName  string `json:"last_name"`
						Username  string `json:"username"`
					} `json:"from"`
					Text  string `json:"text"`
					Photo []struct {
						FileID string `json:"file_id"`
					} `json:"photo"`
				} `json:"message"`
			} `json:"result"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("telegram: unmarshal error: %v", err)
			continue
		}

		if !result.OK {
			log.Printf("telegram: API returned ok=false: %s", string(body))
			continue
		}

		for _, update := range result.Result {
			if update.Message.Chat.ID == 0 {
				continue
			}

			userName := trimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName)
			if userName == "" {
				userName = update.Message.From.Username
			}
			if userName == "" {
				userName = fmt.Sprintf("user_%d", update.Message.From.ID)
			}

			msg := GatewayMessage{
				Platform:  "telegram",
				ChatID:    fmt.Sprintf("%d", update.Message.Chat.ID),
				UserID:    fmt.Sprintf("%d", update.Message.From.ID),
				UserName:  userName,
				Content:   update.Message.Text,
				MsgType:   "text",
				Timestamp: time.Now().Unix(),
			}

			if len(update.Message.Photo) > 0 {
				photo := update.Message.Photo[len(update.Message.Photo)-1]
				msg.Images = []string{photo.FileID}
				msg.MsgType = "image"
			}

			go a.manager.ProcessMessage(msg)

			if update.UpdateID >= int64(offset) {
				offset = int(update.UpdateID) + 1
			}
		}
	}
}

// HandleWebhook handles Telegram webhook POST (alternative to polling)
func (a *TelegramAdapter) HandleWebhook(c *gin.Context) {
	var update struct {
		Message struct {
			Chat struct {
				ID    int64  `json:"id"`
				Title string `json:"title"`
			} `json:"chat"`
			From struct {
				ID        int64  `json:"id"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
				Username  string `json:"username"`
			} `json:"from"`
			Text  string `json:"text"`
			Photo []struct {
				FileID string `json:"file_id"`
			} `json:"photo"`
		} `json:"message"`
	}
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(200, gin.H{"ok": true})
		return
	}

	if update.Message.Chat.ID == 0 {
		c.JSON(200, gin.H{"ok": true})
		return
	}

	userName := trimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName)
	if userName == "" {
		userName = update.Message.From.Username
	}
	if userName == "" {
		userName = fmt.Sprintf("user_%d", update.Message.From.ID)
	}

	msg := GatewayMessage{
		Platform:  "telegram",
		ChatID:    fmt.Sprintf("%d", update.Message.Chat.ID),
		UserID:    fmt.Sprintf("%d", update.Message.From.ID),
		UserName:  userName,
		Content:   update.Message.Text,
		MsgType:   "text",
		Timestamp: time.Now().Unix(),
	}

	if len(update.Message.Photo) > 0 {
		photo := update.Message.Photo[len(update.Message.Photo)-1]
		msg.Images = []string{photo.FileID}
		msg.MsgType = "image"
	}

	go a.manager.ProcessMessage(msg)
	c.JSON(200, gin.H{"ok": true})
}

func (a *TelegramAdapter) SendMessage(chatID, text string) error {
	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIBase, a.token)
	body := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	data, _ := json.Marshal(body)
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram sendMessage: %w", err)
	}
	defer resp.Body.Close()

	// If markdown fails, retry as plain text
	if resp.StatusCode != 200 {
		io.ReadAll(resp.Body)
		resp.Body.Close()
		body["parse_mode"] = ""
		data, _ = json.Marshal(body)
		resp2, err := a.client.Post(url, "application/json", bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("telegram sendMessage (plain): %w", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp2.Body)
			return fmt.Errorf("telegram sendMessage: %s", string(respBody))
		}
	}
	return nil
}

func (a *TelegramAdapter) SendImage(chatID, imageURL string) error {
	url := fmt.Sprintf("%s/bot%s/sendPhoto", telegramAPIBase, a.token)
	body := map[string]any{
		"chat_id": chatID,
		"photo":   imageURL,
	}
	data, _ := json.Marshal(body)
	resp, err := a.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("telegram sendPhoto: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendPhoto: %s", string(respBody))
	}
	return nil
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
