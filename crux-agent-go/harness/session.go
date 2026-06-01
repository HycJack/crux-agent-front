package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-go/core/agent"
)

type SessionTreeEntry struct {
	ID               string              `json:"id"`
	ParentID         string              `json:"parent_id,omitempty"`
	Type             string              `json:"type"`
	Message          *agent.AgentMessage `json:"message,omitempty"`
	Summary          string              `json:"summary,omitempty"`
	Timestamp        string              `json:"timestamp,omitempty"`
	FromID           string              `json:"from_id,omitempty"`
	TokensBefore     int                 `json:"tokens_before,omitempty"`
	FirstKeptEntryID string              `json:"first_kept_entry_id,omitempty"`
	CustomType       string              `json:"custom_type,omitempty"`
	Content          any                 `json:"content,omitempty"`
	Display          bool                `json:"display,omitempty"`
	Details          any                 `json:"details,omitempty"`
}

type SessionMetadata struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Branch    string `json:"branch"`
	CreatedAt string `json:"created_at"`
}

type SessionStorage interface {
	GetMetadata() (*SessionMetadata, error)
	SetMetadata(meta *SessionMetadata) error
	GetEntries() ([]SessionTreeEntry, error)
	AppendEntries(entries []SessionTreeEntry) error
	Close() error
}

type JSONLStorage struct {
	path string
}

func NewJSONLStorage(path string) *JSONLStorage {
	return &JSONLStorage{path: path}
}

func (s *JSONLStorage) GetMetadata() (*SessionMetadata, error) {
	metaPath := s.path + ".meta.json"
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionMetadata{
				ID:        filepath.Base(s.path),
				Branch:    "main",
				CreatedAt: time.Now().Format(time.RFC3339),
			}, nil
		}
		return nil, err
	}
	var meta SessionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *JSONLStorage) SetMetadata(meta *SessionMetadata) error {
	metaPath := s.path + ".meta.json"
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

func (s *JSONLStorage) GetEntries() ([]SessionTreeEntry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []SessionTreeEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry SessionTreeEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *JSONLStorage) AppendEntries(entries []SessionTreeEntry) error {
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (s *JSONLStorage) Close() error { return nil }

type Session struct {
	storage SessionStorage
}

func NewSession(storage SessionStorage) *Session {
	return &Session{storage: storage}
}

func (s *Session) Metadata() (*SessionMetadata, error) {
	return s.storage.GetMetadata()
}

func (s *Session) Entries() ([]SessionTreeEntry, error) {
	return s.storage.GetEntries()
}

func (s *Session) BuildContext() ([]agent.AgentMessage, error) {
	entries, err := s.storage.GetEntries()
	if err != nil {
		return nil, err
	}
	return buildSessionContext(entries)
}

func buildSessionContext(entries []SessionTreeEntry) ([]agent.AgentMessage, error) {
	var messages []agent.AgentMessage
	var compactionIdx int = -1

	for i, entry := range entries {
		if entry.Type == "compaction" {
			compactionIdx = i
		}
	}

	if compactionIdx >= 0 {
		compEntry := entries[compactionIdx]
		messages = append(messages, agent.AgentMessage{
			Role:         "compactionSummary",
			Summary:      compEntry.Summary,
			TokensBefore: compEntry.TokensBefore,
			Timestamp:    time.Now().UnixMilli(),
		})

		foundFirstKept := false
		for i := 0; i < compactionIdx; i++ {
			entry := entries[i]
			if compEntry.FirstKeptEntryID != "" && entry.ID == compEntry.FirstKeptEntryID {
				foundFirstKept = true
			}
			if foundFirstKept && entry.Message != nil {
				messages = append(messages, *entry.Message)
			}
		}

		// FIX: if FirstKeptEntryID not found, include all messages after compaction
		if !foundFirstKept {
			for i := compactionIdx + 1; i < len(entries); i++ {
				entry := entries[i]
				if entry.Message != nil {
					messages = append(messages, *entry.Message)
				}
			}
			return messages, nil
		}

		for i := compactionIdx + 1; i < len(entries); i++ {
			entry := entries[i]
			if entry.Message != nil {
				messages = append(messages, *entry.Message)
			}
		}
	} else {
		for _, entry := range entries {
			if entry.Message != nil {
				messages = append(messages, *entry.Message)
			}
		}
	}

	return messages, nil
}

func (s *Session) AppendMessage(msg agent.AgentMessage) error {
	entry := SessionTreeEntry{
		ID:        generateID(),
		Type:      "message",
		Message:   &msg,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	return s.storage.AppendEntries([]SessionTreeEntry{entry})
}

// FIX: Fork now copies existing entries to the new session.
func (s *Session) Fork(fromEntryID string) (*Session, error) {
	meta, err := s.storage.GetMetadata()
	if err != nil {
		return nil, err
	}
	newMeta := &SessionMetadata{
		ID:        generateID(),
		Title:     meta.Title + " (fork)",
		Branch:    "fork-" + generateID()[:8],
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	// Copy existing entries to new session
	existingEntries, err := s.storage.GetEntries()
	if err != nil {
		return nil, err
	}

	newStorage := &MemoryStorage{
		metadata: newMeta,
		entries:  append([]SessionTreeEntry{}, existingEntries...),
	}
	return NewSession(newStorage), nil
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

type MemoryStorage struct {
	metadata *SessionMetadata
	entries  []SessionTreeEntry
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		metadata: &SessionMetadata{
			ID:        generateID(),
			Branch:    "main",
			CreatedAt: time.Now().Format(time.RFC3339),
		},
	}
}

func (m *MemoryStorage) GetMetadata() (*SessionMetadata, error) { return m.metadata, nil }
func (m *MemoryStorage) SetMetadata(meta *SessionMetadata) error { m.metadata = meta; return nil }
func (m *MemoryStorage) GetEntries() ([]SessionTreeEntry, error) {
	return append([]SessionTreeEntry{}, m.entries...), nil
}
func (m *MemoryStorage) AppendEntries(entries []SessionTreeEntry) error {
	m.entries = append(m.entries, entries...)
	return nil
}
func (m *MemoryStorage) Close() error { return nil }
