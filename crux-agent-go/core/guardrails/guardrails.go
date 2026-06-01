package guardrails

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hermes-go/core/types"
)

// Guardrails provides input/output validation.
type Guardrails struct {
	mu              sync.Mutex
	config          Config
	rateLimiter     map[string]*rateLimit
	blockedPatterns []*regexp.Regexp
}

type Config struct {
	MaxInputLength  int      `yaml:"max_input_length"`
	MaxOutputLength int      `yaml:"max_output_length"`
	BlockedCommands []string `yaml:"blocked_commands"`
	BlockedPatterns []string `yaml:"blocked_patterns"`
	RateLimit       int      `yaml:"rate_limit_per_minute"`
	PIIDetection    bool     `yaml:"pii_detection"`
}

func New(cfg Config) *Guardrails {
	if cfg.MaxInputLength == 0 {
		cfg.MaxInputLength = 100000
	}
	if cfg.MaxOutputLength == 0 {
		cfg.MaxOutputLength = 50000
	}
	g := &Guardrails{
		config:      cfg,
		rateLimiter: make(map[string]*rateLimit),
	}
	for _, p := range cfg.BlockedPatterns {
		if re, err := regexp.Compile(p); err == nil {
			g.blockedPatterns = append(g.blockedPatterns, re)
		}
	}
	return g
}

type rateLimit struct {
	count   int
	resetAt time.Time
}

// CheckInput validates user input.
func (g *Guardrails) CheckInput(input string) error {
	if len(input) > g.config.MaxInputLength {
		return fmt.Errorf("input too long: %d chars (max %d)", len(input), g.config.MaxInputLength)
	}
	for _, re := range g.blockedPatterns {
		if re.MatchString(input) {
			return fmt.Errorf("input contains blocked pattern")
		}
	}
	if g.config.PIIDetection {
		if detectPII(input) {
			return fmt.Errorf("input may contain PII (phone/email/SSN)")
		}
	}
	return nil
}

// CheckOutput validates tool output.
func (g *Guardrails) CheckOutput(name string, result types.ToolResult) types.ToolResult {
	if len(result.Content) > g.config.MaxOutputLength {
		result.Content = result.Content[:g.config.MaxOutputLength] + "\n...(truncated by guardrails)"
	}
	return result
}

// CheckToolCall validates a tool call before execution.
func (g *Guardrails) CheckToolCall(call types.ToolCall) error {
	if call.Function.Name == "exec" {
		var args map[string]any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err == nil {
			if cmd, ok := args["command"].(string); ok {
				for _, blocked := range g.config.BlockedCommands {
					if strings.Contains(cmd, blocked) {
						return fmt.Errorf("command contains blocked pattern: %s", blocked)
					}
				}
			}
		}
	}

	if g.config.RateLimit > 0 {
		g.mu.Lock()
		defer g.mu.Unlock()
		rl, ok := g.rateLimiter[call.Function.Name]
		now := time.Now()
		if !ok || now.After(rl.resetAt) {
			rl = &rateLimit{resetAt: now.Add(time.Minute)}
			g.rateLimiter[call.Function.Name] = rl
		}
		rl.count++
		if rl.count > g.config.RateLimit {
			return fmt.Errorf("rate limit exceeded for %s: %d/min (max %d)", call.Function.Name, rl.count, g.config.RateLimit)
		}
	}

	return nil
}

var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	regexp.MustCompile(`\b\d{10,11}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
}

func detectPII(text string) bool {
	for _, re := range piiPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}
