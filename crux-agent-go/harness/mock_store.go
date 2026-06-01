package harness

import (
	"fmt"
	"sync"
	"time"

	"github.com/hermes-go/core/store"
)

// MockStore is an in-memory store for testing.
type MockStore struct {
	mu       sync.Mutex
	sessions map[string]*store.SessionMeta
	messages map[string][]*store.StoredMessage
	kv       map[string]map[string][]byte
}

func NewMockStore() *MockStore {
	return &MockStore{
		sessions: make(map[string]*store.SessionMeta),
		messages: make(map[string][]*store.StoredMessage),
		kv:       make(map[string]map[string][]byte),
	}
}

func (m *MockStore) CreateSession(sess *store.SessionMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now()
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = time.Now()
	}
	m.sessions[sess.ID] = sess
	return nil
}

func (m *MockStore) GetSession(id string) (*store.SessionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	return sess, nil
}

func (m *MockStore) ListSessions(limit int) ([]store.SessionMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.SessionMeta
	for _, s := range m.sessions {
		result = append(result, *s)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *MockStore) UpdateSession(id string, patch map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	if v, ok := patch["title"].(string); ok {
		sess.Title = v
	}
	if v, ok := patch["message_count"].(int); ok {
		sess.MessageCount = v
	}
	sess.UpdatedAt = time.Now()
	return nil
}

func (m *MockStore) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	delete(m.messages, id)
	return nil
}

func (m *MockStore) AppendMessage(sessionID string, msg *store.StoredMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	msg.SessionID = sessionID
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	if sess, ok := m.sessions[sessionID]; ok {
		sess.MessageCount++
		sess.TokenCount += msg.TokenEst
		sess.UpdatedAt = time.Now()
	}
	return nil
}

func (m *MockStore) GetMessages(sessionID string, opts store.MessageOpts) ([]store.StoredMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.messages[sessionID]
	var result []store.StoredMessage
	for _, msg := range msgs {
		if opts.ExcludeCompacted && msg.Compacted {
			continue
		}
		result = append(result, *msg)
	}
	if opts.Limit > 0 && len(result) > opts.Limit {
		result = result[:opts.Limit]
	}
	return result, nil
}

func (m *MockStore) GetMessage(id string) (*store.StoredMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msgs := range m.messages {
		for _, msg := range msgs {
			if msg.ID == id {
				return msg, nil
			}
		}
	}
	return nil, fmt.Errorf("message %s not found", id)
}

func (m *MockStore) CountMessages(sessionID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages[sessionID]), nil
}

func (m *MockStore) MarkCompacted(sessionID string, beforeID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, msg := range m.messages[sessionID] {
		if !msg.Compacted {
			msg.Compacted = true
			count++
		}
	}
	return count, nil
}

func (m *MockStore) SearchMessages(query string, opts store.SearchOpts) ([]store.SearchHit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var hits []store.SearchHit
	for sid, msgs := range m.messages {
		if opts.SessionID != "" && sid != opts.SessionID {
			continue
		}
		for _, msg := range msgs {
			if contains(msg.Content, query) {
				hits = append(hits, store.SearchHit{
					SessionID: sid,
					MessageID: msg.ID,
					Role:      msg.Role,
					Content:   msg.Content,
					Snippet:   truncate(msg.Content, 128),
				})
			}
		}
	}
	if opts.Limit > 0 && len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	return hits, nil
}

func (m *MockStore) Set(namespace, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kv[namespace] == nil {
		m.kv[namespace] = make(map[string][]byte)
	}
	m.kv[namespace][key] = value
	return nil
}

func (m *MockStore) Get(namespace, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ns, ok := m.kv[namespace]
	if !ok {
		return nil, fmt.Errorf("namespace %s not found", namespace)
	}
	v, ok := ns[key]
	if !ok {
		return nil, fmt.Errorf("key %s not found", key)
	}
	return v, nil
}

func (m *MockStore) Delete(namespace, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ns, ok := m.kv[namespace]; ok {
		delete(ns, key)
	}
	return nil
}

func (m *MockStore) List(namespace string) ([]store.KVPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pairs []store.KVPair
	if ns, ok := m.kv[namespace]; ok {
		for k, v := range ns {
			pairs = append(pairs, store.KVPair{Key: k, Value: v, UpdatedAt: time.Now()})
		}
	}
	return pairs, nil
}

func (m *MockStore) Close() error { return nil }

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
