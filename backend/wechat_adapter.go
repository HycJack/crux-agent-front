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
)

// ════════════════════════════════════════════════
// WeChat (iLink Bot API) Adapter
// ════════════════════════════════════════════════

const (
	ilinkBase     = "https://ilinkai.weixin.qq.com"
	epGetUpdates  = "ilink/bot/getupdates"
	epSendMessage = "ilink/bot/sendmessage"
	epGetBotQR    = "ilink/bot/get_bot_qrcode"
	epGetQRStatus = "ilink/bot/get_qrcode_status"
)

type WeChatAdapter struct {
	botToken   string
	sessionKey string
	manager    *GatewayManager
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
	mu         sync.Mutex
	qrCodeURL  string
	qrToken    string
	loggedIn   bool
	contactID  string
	client     *http.Client
}

func NewWeChatAdapter(botToken, sessionKey string, manager *GatewayManager) *WeChatAdapter {
	return &WeChatAdapter{
		botToken:   botToken,
		sessionKey: sessionKey,
		manager:    manager,
		client:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (a *WeChatAdapter) Name() string { return "wechat" }

func (a *WeChatAdapter) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *WeChatAdapter) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.running = true
	go a.pollLoop()
	log.Printf("wechat: polling started")
	return nil
}

func (a *WeChatAdapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running {
		return nil
	}
	a.cancel()
	a.running = false
	log.Printf("wechat: stopped")
	return nil
}

func (a *WeChatAdapter) pollLoop() {
	backoff := 1 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		body := map[string]any{
			"bot_token": a.botToken,
			"timeout":   35000,
		}
		if a.sessionKey != "" {
			body["session_key"] = a.sessionKey
		}
		data, _ := json.Marshal(body)

		resp, err := a.client.Post(ilinkBase+"/"+epGetUpdates, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Printf("wechat: poll error: %v", err)
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

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("wechat: read body error: %v", err)
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		var result struct {
			ErrCode    int    `json:"errcode"`
			ErrMsg     string `json:"errmsg"`
			SessionKey string `json:"session_key"`
			Updates    []struct {
				MsgType    string `json:"msg_type"`
				MsgID      string `json:"msg_id"`
				FromUser   string `json:"from_user"`
				ToUser     string `json:"to_user"`
				Content    string `json:"content"`
				CreateTime int64  `json:"create_time"`
				MsgDataID  string `json:"msg_data_id"`
			} `json:"updates"`
		}

		if err := json.Unmarshal(respBody, &result); err != nil {
			log.Printf("wechat: unmarshal error: %v, body: %s", err, string(respBody))
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		// Handle stale session or errors
		if result.ErrCode == -14 || (result.ErrCode == -2 && result.ErrMsg == "unknown error") {
			log.Printf("wechat: stale session, need re-login (errcode=%d)", result.ErrCode)
			a.mu.Lock()
			a.loggedIn = false
			a.sessionKey = ""
			a.mu.Unlock()
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

		if result.ErrCode != 0 && result.ErrCode != -1 {
			log.Printf("wechat: getupdates error: %d %s", result.ErrCode, result.ErrMsg)
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}

		// Store session key if returned
		if result.SessionKey != "" {
			a.mu.Lock()
			a.sessionKey = result.SessionKey
			a.loggedIn = true
			a.mu.Unlock()
		}

		backoff = 1 * time.Second // reset on success

		for _, update := range result.Updates {
			if update.MsgType != "text" && update.MsgType != "image" {
				continue
			}

			msg := GatewayMessage{
				Platform:  "wechat",
				ChatID:    update.FromUser,
				UserID:    update.FromUser,
				UserName:  update.FromUser,
				Content:   update.Content,
				MsgType:   update.MsgType,
				Timestamp: time.Now().Unix(),
			}

			go a.manager.ProcessMessage(msg)
		}
	}
}

func (a *WeChatAdapter) SendMessage(chatID, text string) error {
	a.mu.Lock()
	sessionKey := a.sessionKey
	botToken := a.botToken
	a.mu.Unlock()

	body := map[string]any{
		"bot_token":   botToken,
		"session_key": sessionKey,
		"to_user":     chatID,
		"msg_type":    "text",
		"content":     text,
	}
	data, _ := json.Marshal(body)

	resp, err := a.client.Post(ilinkBase+"/"+epSendMessage, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("wechat sendMessage: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	json.Unmarshal(respBody, &result)
	if result.ErrCode != 0 {
		return fmt.Errorf("wechat sendMessage: %d %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

func (a *WeChatAdapter) SendImage(chatID, imageURL string) error {
	// iLink supports image msg_type with media_id or url
	// For simplicity, send as text with URL
	return a.SendMessage(chatID, "[Image] "+imageURL)
}

// GetQRCode requests a QR code URL for WeChat login via iLink Bot API
func (a *WeChatAdapter) GetQRCode() (string, error) {
	body := map[string]string{
		"bot_token": a.botToken,
	}
	data, _ := json.Marshal(body)

	resp, err := a.client.Post(ilinkBase+"/"+epGetBotQR, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("wechat getQRCode: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		QRCode  string `json:"qrcode"`
		Token   string `json:"token"`
	}
	json.Unmarshal(respBody, &result)
	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat getQRCode: %d %s", result.ErrCode, result.ErrMsg)
	}

	a.mu.Lock()
	a.qrCodeURL = result.QRCode
	a.qrToken = result.Token
	a.mu.Unlock()

	return result.QRCode, nil
}

// CheckQRStatus checks whether the QR code was scanned and login completed
func (a *WeChatAdapter) CheckQRStatus() (string, string, error) {
	a.mu.Lock()
	qrToken := a.qrToken
	botToken := a.botToken
	a.mu.Unlock()

	if qrToken == "" {
		return "", "", fmt.Errorf("no QR code token, call GetQRCode first")
	}

	body := map[string]string{
		"bot_token": botToken,
		"token":     qrToken,
	}
	data, _ := json.Marshal(body)

	resp, err := a.client.Post(ilinkBase+"/"+epGetQRStatus, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", "", fmt.Errorf("wechat checkQRStatus: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
		Status     string `json:"status"` // "scanned", "confirmed", "expired"
		SessionKey string `json:"session_key"`
	}
	json.Unmarshal(respBody, &result)
	if result.ErrCode != 0 {
		return "", "", fmt.Errorf("wechat checkQRStatus: %d %s", result.ErrCode, result.ErrMsg)
	}

	// If login confirmed, store session key
	if result.Status == "confirmed" && result.SessionKey != "" {
		a.mu.Lock()
		a.sessionKey = result.SessionKey
		a.loggedIn = true
		a.mu.Unlock()
	}

	return result.Status, result.SessionKey, nil
}
