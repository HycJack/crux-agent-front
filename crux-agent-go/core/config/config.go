package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent  AgentConfig `yaml:"agent"`
	LLM    LLMConfig   `yaml:"llm"`
	Store  StoreConfig `yaml:"store"`
	Skills []string    `yaml:"skills"`
}

type AgentConfig struct {
	Name         string `yaml:"name"`
	SystemPrompt string `yaml:"system_prompt"`
}

type LLMConfig struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	APIKey      string  `yaml:"api_key"`
	BaseURL     string  `yaml:"base_url"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	MaxRounds   int     `yaml:"max_rounds"`
}

type StoreConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Env overrides
	if cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = os.Getenv("HERMES_API_KEY")
	}
	if cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	// Defaults
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "gpt-4o"
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 4096
	}
	if cfg.LLM.MaxRounds == 0 {
		cfg.LLM.MaxRounds = 20
	}
	if cfg.LLM.Temperature == 0 {
		cfg.LLM.Temperature = 0.7
	}
	if cfg.Store.Driver == "" {
		cfg.Store.Driver = "sqlite"
	}
	if cfg.Store.DSN == "" {
		home, _ := os.UserHomeDir()
		cfg.Store.DSN = filepath.Join(home, ".hermes", "data", "hermes.db")
	}
	// Expand ~ in DSN
	if strings.HasPrefix(cfg.Store.DSN, "~/") {
		home, _ := os.UserHomeDir()
		cfg.Store.DSN = filepath.Join(home, cfg.Store.DSN[2:])
	}
	if cfg.Agent.Name == "" {
		cfg.Agent.Name = "hermes"
	}

	return cfg, nil
}
