package harness

import (
	"fmt"
	"strings"
	"time"

	"github.com/hermes-go/core/context"
	"github.com/hermes-go/core/types"
)

// EvalResult holds evaluation scores for a scenario.
type EvalResult struct {
	Scenario    string
	Correctness float64
	Efficiency  float64
	Safety      float64
	Latency     time.Duration
	TokenUsage  int
	Details     []string
}

// Evaluator scores scenario results.
type Evaluator struct {
	rules []Rule
}

// Rule defines an evaluation criterion.
type Rule struct {
	Name    string
	Weight  float64
	Evaluate func(*ScenarioResult) (float64, string)
}

func NewEvaluator() *Evaluator {
	return &Evaluator{
		rules: []Rule{
			{Name: "no_error", Weight: 0.3, Evaluate: evalNoError},
			{Name: "efficiency", Weight: 0.2, Evaluate: evalEfficiency},
			{Name: "tool_usage", Weight: 0.3, Evaluate: evalToolUsage},
			{Name: "response_quality", Weight: 0.2, Evaluate: evalResponseQuality},
		},
	}
}

func (e *Evaluator) Evaluate(result *ScenarioResult) *EvalResult {
	eval := &EvalResult{
		Scenario: result.Scenario,
	}

	totalWeight := 0.0
	for _, rule := range e.rules {
		score, detail := rule.Evaluate(result)
		eval.Correctness += score * rule.Weight
		totalWeight += rule.Weight
		if detail != "" {
			eval.Details = append(eval.Details, detail)
		}
	}

	if totalWeight > 0 {
		eval.Correctness /= totalWeight
	}

	// Token usage
	for _, msg := range result.Messages {
		eval.TokenUsage += context.EstimateTokens([]types.Message{msg})
	}

	return eval
}

func evalNoError(result *ScenarioResult) (float64, string) {
	if result.Error != nil {
		return 0.0, fmt.Sprintf("error: %v", result.Error)
	}
	return 1.0, ""
}

func evalEfficiency(result *ScenarioResult) (float64, string) {
	// Fewer rounds = more efficient
	if result.Rounds <= 1 {
		return 1.0, ""
	}
	if result.Rounds <= 3 {
		return 0.8, ""
	}
	if result.Rounds <= 5 {
		return 0.6, ""
	}
	return 0.4, fmt.Sprintf("many rounds: %d", result.Rounds)
}

func evalToolUsage(result *ScenarioResult) (float64, string) {
	if len(result.ToolCalls) == 0 {
		return 0.5, "no tool calls"
	}
	return 1.0, ""
}

func evalResponseQuality(result *ScenarioResult) (float64, string) {
	for _, msg := range result.Messages {
		if msg.Role == "assistant" && msg.Content != "" {
			if len(msg.Content) > 10 {
				return 1.0, ""
			}
			return 0.5, "short response"
		}
	}
	return 0.0, "no response"
}

// Summary returns a human-readable summary of an evaluation.
func (e *EvalResult) Summary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Scenario: %s\n", e.Scenario))
	sb.WriteString(fmt.Sprintf("  Correctness: %.2f\n", e.Correctness))
	sb.WriteString(fmt.Sprintf("  Tokens: %d\n", e.TokenUsage))
	if len(e.Details) > 0 {
		sb.WriteString("  Details:\n")
		for _, d := range e.Details {
			sb.WriteString(fmt.Sprintf("    - %s\n", d))
		}
	}
	return sb.String()
}
