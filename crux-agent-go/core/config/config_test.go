package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	content := `
agent:
  name: test-agent
  system_prompt: You are a test agent.
llm:
  provider: openai
  model: gpt-4o-mini
  api_key: sk-test-key
  base_url: https://api.openai.com/v1
  max_tokens: 2048
  temperature: 0.5
  max_rounds: 10
store:
  driver: sqlite
  dsn: /tmp/test-hermes.db
skills:
  - terminal
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Agent.Name != "test-agent" {
		t.Errorf("name: got %s", cfg.Agent.Name)
	}
	if cfg.LLM.Model != "gpt-4o-mini" {
		t.Errorf("model: got %s", cfg.LLM.Model)
	}
	if cfg.LLM.APIKey != "sk-test-key" {
		t.Errorf("api_key: got %s", cfg.LLM.APIKey)
	}
	if cfg.LLM.MaxTokens != 2048 {
		t.Errorf("max_tokens: got %d", cfg.LLM.MaxTokens)
	}
	if cfg.LLM.MaxRounds != 10 {
		t.Errorf("max_rounds: got %d", cfg.LLM.MaxRounds)
	}
	if cfg.Store.Driver != "sqlite" {
		t.Errorf("store driver: got %s", cfg.Store.Driver)
	}
	if len(cfg.Skills) != 1 || cfg.Skills[0] != "terminal" {
		t.Errorf("skills: got %v", cfg.Skills)
	}
}

func TestLoadDefaults(t *testing.T) {
	content := `
agent:
  name: minimal
  system_prompt: test
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.LLM.Model != "gpt-4o" {
		t.Errorf("default model: got %s", cfg.LLM.Model)
	}
	if cfg.LLM.MaxTokens != 4096 {
		t.Errorf("default max_tokens: got %d", cfg.LLM.MaxTokens)
	}
	if cfg.LLM.MaxRounds != 20 {
		t.Errorf("default max_rounds: got %d", cfg.LLM.MaxRounds)
	}
	if cfg.LLM.Temperature != 0.7 {
		t.Errorf("default temperature: got %f", cfg.LLM.Temperature)
	}
	if cfg.Store.Driver != "sqlite" {
		t.Errorf("default store driver: got %s", cfg.Store.Driver)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	content := `
agent:
  name: env-test
  system_prompt: test
llm:
  api_key: ""
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(content), 0644)

	os.Setenv("HERMES_API_KEY", "sk-from-env")
	defer os.Unsetenv("HERMES_API_KEY")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.LLM.APIKey != "sk-from-env" {
		t.Errorf("env api_key: got %s", cfg.LLM.APIKey)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("{{invalid yaml"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}
