package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// ════════════════════════════════════════════════
// Gateway Config Store (JSON-backed, editable from web UI)
// ════════════════════════════════════════════════

type PlatformConfig struct {
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings"` // platform-specific key-value pairs
}

type GatewayConfigStore struct {
	path   string
	mu     sync.RWMutex
	config map[string]*PlatformConfig // platform name -> config
}

func NewGatewayConfigStore(path string) *GatewayConfigStore {
	s := &GatewayConfigStore{
		path:   path,
		config: make(map[string]*PlatformConfig),
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	s.load()
	return s
}

func (s *GatewayConfigStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var items map[string]*PlatformConfig
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("GatewayConfigStore load error: %v", err)
		return
	}
	if items != nil {
		s.config = items
	}
}

func (s *GatewayConfigStore) save() {
	data, _ := json.MarshalIndent(s.config, "", "  ")
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("GatewayConfigStore save error: %v", err)
	}
}

// Get returns the config for a platform, or nil if not found.
func (s *GatewayConfigStore) Get(platform string) *PlatformConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config[platform]
}

// Set creates or updates the config for a platform.
func (s *GatewayConfigStore) Set(platform string, cfg *PlatformConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config[platform] = cfg
	s.save()
}

// List returns all platform configs.
func (s *GatewayConfigStore) List() map[string]*PlatformConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*PlatformConfig, len(s.config))
	for k, v := range s.config {
		result[k] = v
	}
	return result
}

// Delete removes a platform config.
func (s *GatewayConfigStore) Delete(platform string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.config[platform]; !ok {
		return false
	}
	delete(s.config, platform)
	s.save()
	return true
}

// EnabledPlatforms returns a list of platform names that are enabled.
func (s *GatewayConfigStore) EnabledPlatforms() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []string
	for name, cfg := range s.config {
		if cfg.Enabled {
			result = append(result, name)
		}
	}
	return result
}
