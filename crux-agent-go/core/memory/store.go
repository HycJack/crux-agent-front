package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	dir     string
	entries []Entry
}

type Entry struct {
	Category  string    `json:"category"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Save(category, key, value string) error {
	found := false
	for i, e := range s.entries {
		if e.Key == key {
			s.entries[i].Value = value
			s.entries[i].Category = category
			s.entries[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		s.entries = append(s.entries, Entry{
			Category:  category,
			Key:       key,
			Value:     value,
			UpdatedAt: time.Now(),
		})
	}
	return s.save()
}

func (s *Store) Search(query string, category string) []Entry {
	var results []Entry
	q := strings.ToLower(query)
	for _, e := range s.entries {
		if category != "" && e.Category != category {
			continue
		}
		if strings.Contains(strings.ToLower(e.Key), q) ||
			strings.Contains(strings.ToLower(e.Value), q) {
			results = append(results, e)
		}
	}
	return results
}

func (s *Store) List(category string) []Entry {
	var results []Entry
	for _, e := range s.entries {
		if category == "" || e.Category == category {
			results = append(results, e)
		}
	}
	return results
}

func (s *Store) Delete(key string) error {
	for i, e := range s.entries {
		if e.Key == key {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("memory %q not found", key)
}

// GetText returns all memories formatted for context injection.
// FIX: creates a copy before sorting to avoid modifying the original slice.
func (s *Store) GetText() string {
	if len(s.entries) == 0 {
		return ""
	}
	// Create a copy to avoid modifying the original slice
	sorted := make([]Entry, len(s.entries))
	copy(sorted, s.entries)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Category != sorted[j].Category {
			return sorted[i].Category < sorted[j].Category
		}
		return sorted[i].Key < sorted[j].Key
	})

	var sb strings.Builder
	currentCat := ""
	for _, e := range sorted {
		if e.Category != currentCat {
			currentCat = e.Category
			sb.WriteString(fmt.Sprintf("\n[%s]\n", currentCat))
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", e.Key, e.Value))
	}
	return sb.String()
}

func (s *Store) WriteMemoryFile() error {
	path := filepath.Join(s.dir, "MEMORY.md")
	var sb strings.Builder
	sb.WriteString("# Agent Memory\n\n")
	for _, e := range s.entries {
		if e.Category == "memory" {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Key, e.Value))
		}
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func (s *Store) WriteUserFile() error {
	path := filepath.Join(s.dir, "USER.md")
	var sb strings.Builder
	sb.WriteString("# User Profile\n\n")
	for _, e := range s.entries {
		if e.Category == "user" {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", e.Key, e.Value))
		}
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func (s *Store) load() error {
	path := filepath.Join(s.dir, "memories.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.entries)
}

// save persists entries and writes markdown files.
// FIX: log errors from WriteMemoryFile/WriteUserFile instead of silently ignoring.
func (s *Store) save() error {
	path := filepath.Join(s.dir, "memories.json")
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	// Best-effort markdown files
	if err := s.WriteMemoryFile(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: write MEMORY.md: %v\n", err)
	}
	if err := s.WriteUserFile(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: write USER.md: %v\n", err)
	}
	return nil
}
