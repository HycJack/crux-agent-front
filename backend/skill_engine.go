package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hermes-go/core/llm"
	"github.com/hermes-go/core/types"
)

// ════════════════════════════════════════════════
// Runtime Skill Data Model
// ════════════════════════════════════════════════

type RuntimeSkill struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Trigger     string            `json:"trigger"`
	Steps       []SkillStep       `json:"steps"`
	Tools       []string          `json:"tools"`
	EnvVars     map[string]string `json:"env_vars"`
	Prompt      string            `json:"prompt"`

	// Lifecycle
	State     string    `json:"state"`       // active, stale, archived, pinned
	Version   int       `json:"version"`
	CreatedBy string    `json:"created_by"`  // "runtime", "agent", "user"
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Usage stats
	UseCount    int       `json:"use_count"`
	PatchCount  int       `json:"patch_count"`
	FailCount   int       `json:"fail_count"`
	LastUsed    time.Time `json:"last_used"`
	LastPatched time.Time `json:"last_patched"`

	// Source tracking
	SourceSession string `json:"source_session"`
}

type SkillStep struct {
	Index    int                    `json:"index"`
	Tool     string                 `json:"tool"`
	Args     map[string]interface{} `json:"args,omitempty"`
	Purpose  string                 `json:"purpose"`
	Optional bool                   `json:"optional"`
}

// ════════════════════════════════════════════════
// Runtime Skill Store
// ════════════════════════════════════════════════

type RuntimeSkillStore struct {
	path   string
	mu     sync.RWMutex
	skills map[string]*RuntimeSkill
}

func NewRuntimeSkillStore(path string) *RuntimeSkillStore {
	s := &RuntimeSkillStore{path: path, skills: make(map[string]*RuntimeSkill)}
	os.MkdirAll(filepath.Dir(path), 0o755)
	s.load()
	return s
}

func (s *RuntimeSkillStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var items []*RuntimeSkill
	if err := json.Unmarshal(data, &items); err != nil {
		log.Printf("RuntimeSkillStore load error: %v", err)
		return
	}
	for _, sk := range items {
		s.skills[sk.ID] = sk
	}
}

func (s *RuntimeSkillStore) save() {
	list := make([]*RuntimeSkill, 0, len(s.skills))
	for _, sk := range s.skills {
		list = append(list, sk)
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("RuntimeSkillStore save error: %v", err)
	}
}

func (s *RuntimeSkillStore) Get(id string) *RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skills[id]
}

func (s *RuntimeSkillStore) List() []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*RuntimeSkill, 0, len(s.skills))
	for _, sk := range s.skills {
		result = append(result, sk)
	}
	return result
}

func (s *RuntimeSkillStore) Create(sk *RuntimeSkill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sk.ID == "" {
		sk.ID = fmt.Sprintf("rtskill-%d", time.Now().UnixNano())
	}
	now := time.Now()
	sk.CreatedAt = now
	sk.UpdatedAt = now
	if sk.Version == 0 {
		sk.Version = 1
	}
	if sk.State == "" {
		sk.State = "active"
	}
	s.skills[sk.ID] = sk
	s.save()
	return nil
}

func (s *RuntimeSkillStore) Update(sk *RuntimeSkill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.skills[sk.ID]
	if !ok {
		return fmt.Errorf("not found")
	}
	sk.CreatedAt = existing.CreatedAt
	sk.CreatedBy = existing.CreatedBy
	sk.SourceSession = existing.SourceSession
	sk.UpdatedAt = time.Now()
	s.skills[sk.ID] = sk
	s.save()
	return nil
}

func (s *RuntimeSkillStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.skills[id]; !ok {
		return fmt.Errorf("not found")
	}
	delete(s.skills, id)
	s.save()
	return nil
}

func (s *RuntimeSkillStore) ListByState(state string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*RuntimeSkill, 0)
	for _, sk := range s.skills {
		if sk.State == state {
			result = append(result, sk)
		}
	}
	return result
}

func (s *RuntimeSkillStore) ListByTool(toolName string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*RuntimeSkill, 0)
	for _, sk := range s.skills {
		for _, t := range sk.Tools {
			if t == toolName {
				result = append(result, sk)
				break
			}
		}
	}
	return result
}

func (s *RuntimeSkillStore) Search(query string) []*RuntimeSkill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(query)
	result := make([]*RuntimeSkill, 0)
	for _, sk := range s.skills {
		if strings.Contains(strings.ToLower(sk.Name), q) ||
			strings.Contains(strings.ToLower(sk.Description), q) ||
			strings.Contains(strings.ToLower(sk.Trigger), q) {
			result = append(result, sk)
		}
	}
	return result
}

// ════════════════════════════════════════════════
// Session Trace / Tracker
// ════════════════════════════════════════════════

type TrackedCall struct {
	Index    int             `json:"index"`
	Tool     string          `json:"tool"`
	Args     json.RawMessage `json:"args"`
	Result   string          `json:"result"`
	Duration time.Duration   `json:"duration"`
	Time     time.Time       `json:"time"`
}

type SessionTrace struct {
	SessionID string        `json:"session_id"`
	StartTime time.Time     `json:"start_time"`
	Calls     []TrackedCall `json:"calls"`
	Done      bool          `json:"done"`
}

type SkillTracker struct {
	mu       sync.Mutex
	sessions map[string]*SessionTrace
}

func NewSkillTracker() *SkillTracker {
	return &SkillTracker{sessions: make(map[string]*SessionTrace)}
}

func (t *SkillTracker) StartSession(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions[sessionID] = &SessionTrace{
		SessionID: sessionID,
		StartTime: time.Now(),
		Calls:     make([]TrackedCall, 0),
	}
}

func (t *SkillTracker) RecordCall(sessionID string, call TrackedCall) {
	t.mu.Lock()
	defer t.mu.Unlock()
	trace, ok := t.sessions[sessionID]
	if !ok {
		trace = &SessionTrace{
			SessionID: sessionID,
			StartTime: time.Now(),
			Calls:     make([]TrackedCall, 0),
		}
		t.sessions[sessionID] = trace
	}
	call.Index = len(trace.Calls)
	trace.Calls = append(trace.Calls, call)
}

func (t *SkillTracker) EndSession(sessionID string) *SessionTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	trace, ok := t.sessions[sessionID]
	if !ok {
		return nil
	}
	trace.Done = true
	delete(t.sessions, sessionID)
	return trace
}

func (t *SkillTracker) GetTrace(sessionID string) *SessionTrace {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessions[sessionID]
}

func (t *SkillTracker) Cleanup(olderThan time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	for id, trace := range t.sessions {
		if trace.StartTime.Before(cutoff) {
			delete(t.sessions, id)
		}
	}
}

// ════════════════════════════════════════════════
// Skill Matcher
// ════════════════════════════════════════════════

type MatchResult struct {
	Skill     *RuntimeSkill
	Deviation float64
}

type SkillMatcher struct {
	store *RuntimeSkillStore
}

func NewSkillMatcher(store *RuntimeSkillStore) *SkillMatcher {
	return &SkillMatcher{store: store}
}

func (m *SkillMatcher) FindMatchingSkills(trace []TrackedCall) []*MatchResult {
	if len(trace) == 0 {
		return nil
	}
	traceTools := make([]string, 0, len(trace))
	for _, c := range trace {
		traceTools = append(traceTools, c.Tool)
	}

	skills := m.store.List()
	var results []*MatchResult
	for _, sk := range skills {
		if sk.State == "archived" {
			continue
		}
		overlap := toolOverlap(traceTools, sk.Tools)
		if overlap > 0.6 {
			dev := m.CalculateDeviation(trace, sk)
			results = append(results, &MatchResult{Skill: sk, Deviation: dev})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Deviation < results[j].Deviation })
	return results
}

func (m *SkillMatcher) CalculateDeviation(trace []TrackedCall, skill *RuntimeSkill) float64 {
	if len(skill.Steps) == 0 {
		return 1.0
	}
	traceTools := make([]string, len(trace))
	for i, c := range trace {
		traceTools[i] = c.Tool
	}
	skillTools := make([]string, len(skill.Steps))
	for i, s := range skill.Steps {
		skillTools[i] = s.Tool
	}

	// Count ordered matches (same tool in same relative position)
	matched := 0
	maxLen := len(skillTools)
	if len(traceTools) > maxLen {
		maxLen = len(traceTools)
	}
	minLen := len(skillTools)
	if len(traceTools) < minLen {
		minLen = len(traceTools)
	}
	for i := 0; i < minLen; i++ {
		if traceTools[i] == skillTools[i] {
			matched++
		}
	}
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(matched)/float64(maxLen)
}

func toolOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setB := make(map[string]bool, len(b))
	for _, t := range b {
		setB[t] = true
	}
	common := 0
	for _, t := range a {
		if setB[t] {
			common++
		}
	}
	total := len(a)
	if len(b) > total {
		total = len(b)
	}
	return float64(common) / float64(total)
}

// ════════════════════════════════════════════════
// Skill Extractor
// ════════════════════════════════════════════════

type SkillExtractor struct {
	provider llm.Provider
	store    *RuntimeSkillStore
}

func NewSkillExtractor(provider llm.Provider, store *RuntimeSkillStore) *SkillExtractor {
	return &SkillExtractor{provider: provider, store: store}
}

func (e *SkillExtractor) ExtractFromTrace(trace *SessionTrace) *RuntimeSkill {
	if trace == nil || len(trace.Calls) < 5 {
		return nil
	}

	// Build a summary of the call sequence for the LLM
	var lines []string
	for _, c := range trace.Calls {
		argsPreview := string(c.Args)
		if len(argsPreview) > 200 {
			argsPreview = argsPreview[:200] + "..."
		}
		resultPreview := c.Result
		if len(resultPreview) > 100 {
			resultPreview = resultPreview[:100] + "..."
		}
		lines = append(lines, fmt.Sprintf("%d. %s(%s) → %s", c.Index+1, c.Tool, argsPreview, resultPreview))
	}

	prompt := fmt.Sprintf(`Given this sequence of tool calls from a completed task, extract a reusable procedure.

Sequence:
%s

Generate a JSON object with these fields (reply with ONLY valid JSON, no markdown):
{
  "name": "short descriptive name (2-4 words)",
  "description": "what this procedure does in one sentence",
  "trigger": "natural language description of when to use this procedure",
  "steps": [
    {"index": 0, "tool": "tool_name", "purpose": "why this step", "optional": false}
  ],
  "tools": ["tool_name1", "tool_name2"],
  "prompt": "system prompt addition for this procedure"
}`, strings.Join(lines, "\n"))

	messages := []types.Message{
		{Role: "user", Content: prompt},
	}
	resp, err := e.provider.Complete(messages, nil)
	if err != nil {
		log.Printf("[SkillExtractor] LLM call failed: %v", err)
		return nil
	}

	// Parse response - try to extract JSON from response
	content := resp.Message.Content
	content = extractJSON(content)
	if content == "" {
		log.Printf("[SkillExtractor] no JSON in response")
		return nil
	}

	var draft struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Trigger     string      `json:"trigger"`
		Steps       []SkillStep `json:"steps"`
		Tools       []string    `json:"tools"`
		Prompt      string      `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(content), &draft); err != nil {
		log.Printf("[SkillExtractor] JSON parse failed: %v", err)
		return nil
	}

	if draft.Name == "" {
		return nil
	}

	// Build tools list from trace if not provided
	if len(draft.Tools) == 0 {
		toolSet := make(map[string]bool)
		for _, c := range trace.Calls {
			toolSet[c.Tool] = true
		}
		for t := range toolSet {
			draft.Tools = append(draft.Tools, t)
		}
	}

	skill := &RuntimeSkill{
		ID:            fmt.Sprintf("rtskill-%d", time.Now().UnixNano()),
		Name:          draft.Name,
		Description:   draft.Description,
		Trigger:       draft.Trigger,
		Steps:         draft.Steps,
		Tools:         draft.Tools,
		Prompt:        draft.Prompt,
		State:         "active",
		Version:       1,
		CreatedBy:     "runtime",
		SourceSession: trace.SessionID,
	}
	return skill
}

func extractJSON(s string) string {
	// Try to find JSON in the string (handles markdown code blocks)
	s = strings.TrimSpace(s)
	// Remove markdown code block wrappers
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// ════════════════════════════════════════════════
// Skill Repairer
// ════════════════════════════════════════════════

type SkillRepairer struct {
	provider llm.Provider
	store    *RuntimeSkillStore
}

func NewSkillRepairer(provider llm.Provider, store *RuntimeSkillStore) *SkillRepairer {
	return &SkillRepairer{provider: provider, store: store}
}

func (r *SkillRepairer) RepairIfNeeded(skill *RuntimeSkill, trace []TrackedCall, deviation float64) bool {
	if deviation < 0.3 {
		// Minor deviation, acceptable
		return false
	}

	if deviation >= 0.7 {
		// Too different, mark stale
		skill.State = "stale"
		skill.UpdatedAt = time.Now()
		r.store.Update(skill)
		log.Printf("[SkillRepairer] skill %s marked stale (deviation=%.2f)", skill.Name, deviation)
		return true
	}

	// deviation >= 0.3 && < 0.7: generate a patch
	var traceLines []string
	for _, c := range trace {
		traceLines = append(traceLines, fmt.Sprintf("%d. %s", c.Index+1, c.Tool))
	}
	var stepLines []string
	for _, s := range skill.Steps {
		stepLines = append(stepLines, fmt.Sprintf("%d. %s (%s)", s.Index+1, s.Tool, s.Purpose))
	}

	prompt := fmt.Sprintf(`The skill "%s" has these steps:
%s

The actual execution was:
%s

What changed? Reply with ONLY a JSON object containing updated steps:
{"steps": [{"index": 0, "tool": "tool_name", "purpose": "why", "optional": false}], "description": "updated description if needed"}`,
		skill.Name,
		strings.Join(stepLines, "\n"),
		strings.Join(traceLines, "\n"))

	messages := []types.Message{{Role: "user", Content: prompt}}
	resp, err := r.provider.Complete(messages, nil)
	if err != nil {
		log.Printf("[SkillRepairer] LLM call failed: %v", err)
		return false
	}

	content := extractJSON(resp.Message.Content)
	if content == "" {
		return false
	}

	var patch struct {
		Steps       []SkillStep `json:"steps"`
		Description string      `json:"description"`
	}
	if err := json.Unmarshal([]byte(content), &patch); err != nil {
		log.Printf("[SkillRepairer] JSON parse failed: %v", err)
		return false
	}

	if len(patch.Steps) > 0 {
		skill.Steps = patch.Steps
	}
	if patch.Description != "" {
		skill.Description = patch.Description
	}
	skill.Version++
	skill.PatchCount++
	skill.LastPatched = time.Now()
	skill.UpdatedAt = time.Now()

	// Update tools list from new steps
	toolSet := make(map[string]bool)
	for _, s := range skill.Steps {
		toolSet[s.Tool] = true
	}
	skill.Tools = make([]string, 0, len(toolSet))
	for t := range toolSet {
		skill.Tools = append(skill.Tools, t)
	}

	r.store.Update(skill)
	log.Printf("[SkillRepairer] patched skill %s to v%d", skill.Name, skill.Version)
	return true
}

// ════════════════════════════════════════════════
// Skill Engine (coordinator)
// ════════════════════════════════════════════════

type SkillEngine struct {
	store       *RuntimeSkillStore
	tracker     *SkillTracker
	extractor   *SkillExtractor
	matcher     *SkillMatcher
	repairer    *SkillRepairer
	llmProvider llm.Provider
}

func NewSkillEngine(provider llm.Provider, storePath string) *SkillEngine {
	store := NewRuntimeSkillStore(storePath)
	return &SkillEngine{
		store:       store,
		tracker:     NewSkillTracker(),
		extractor:   NewSkillExtractor(provider, store),
		matcher:     NewSkillMatcher(store),
		repairer:    NewSkillRepairer(provider, store),
		llmProvider: provider,
	}
}

func (e *SkillEngine) ProcessCompletedTrace(trace *SessionTrace) {
	if trace == nil || len(trace.Calls) < 5 {
		return
	}

	log.Printf("[SkillEngine] processing trace for session %s (%d calls)", trace.SessionID, len(trace.Calls))

	// Check if this matches an existing skill
	matches := e.matcher.FindMatchingSkills(trace.Calls)
	if len(matches) > 0 {
		best := matches[0]
		log.Printf("[SkillEngine] matched skill %s (deviation=%.2f)", best.Skill.Name, best.Deviation)
		// Repair if deviation is significant
		e.repairer.RepairIfNeeded(best.Skill, trace.Calls, best.Deviation)
		return
	}

	// No match found, try to extract a new skill
	skill := e.extractor.ExtractFromTrace(trace)
	if skill != nil {
		if err := e.store.Create(skill); err != nil {
			log.Printf("[SkillEngine] failed to create skill: %v", err)
			return
		}
		log.Printf("[SkillEngine] extracted new skill: %s (%s)", skill.Name, skill.ID)
	}
}

func (e *SkillEngine) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.CleanupStaleSkills()
			e.tracker.Cleanup(24 * time.Hour)
		}
	}
}

func (e *SkillEngine) CleanupStaleSkills() {
	now := time.Now()
	for _, sk := range e.store.List() {
		if sk.State == "pinned" || sk.State == "archived" {
			continue
		}
		if sk.CreatedBy != "runtime" {
			continue
		}

		daysSinceUse := now.Sub(sk.LastUsed).Hours() / 24
		if sk.LastUsed.IsZero() {
			daysSinceUse = now.Sub(sk.CreatedAt).Hours() / 24
		}

		// Skills not used for 90 days and already stale → archive
		if daysSinceUse > 90 && sk.State == "stale" {
			sk.State = "archived"
			sk.UpdatedAt = now
			e.store.Update(sk)
			log.Printf("[SkillEngine] archived skill %s (unused for %.0f days)", sk.Name, daysSinceUse)
			continue
		}

		// Skills not used for 30 days → mark stale
		if daysSinceUse > 30 && sk.State == "active" {
			sk.State = "stale"
			sk.UpdatedAt = now
			e.store.Update(sk)
			log.Printf("[SkillEngine] marked skill %s stale (unused for %.0f days)", sk.Name, daysSinceUse)
			continue
		}

		// Skills with high failure rate → mark stale
		if sk.UseCount > 0 && float64(sk.FailCount) > float64(sk.UseCount)*0.5 && sk.State == "active" {
			sk.State = "stale"
			sk.UpdatedAt = now
			e.store.Update(sk)
			log.Printf("[SkillEngine] marked skill %s stale (high failure rate: %d/%d)", sk.Name, sk.FailCount, sk.UseCount)
		}
	}
}

// ════════════════════════════════════════════════
// API Handlers
// ════════════════════════════════════════════════

func (e *ChatEngine) ListRuntimeSkills(c *gin.Context) {
	state := c.Query("state")
	tool := c.Query("tool")
	query := c.Query("q")

	var skills []*RuntimeSkill
	if query != "" {
		skills = e.runtimeSkillEngine.store.Search(query)
	} else if tool != "" {
		skills = e.runtimeSkillEngine.store.ListByTool(tool)
	} else if state != "" {
		skills = e.runtimeSkillEngine.store.ListByState(state)
	} else {
		skills = e.runtimeSkillEngine.store.List()
	}
	c.JSON(200, gin.H{"skills": skills})
}

func (e *ChatEngine) GetRuntimeSkill(c *gin.Context) {
	id := c.Param("id")
	skill := e.runtimeSkillEngine.store.Get(id)
	if skill == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, skill)
}

func (e *ChatEngine) UpdateRuntimeSkill(c *gin.Context) {
	id := c.Param("id")
	existing := e.runtimeSkillEngine.store.Get(id)
	if existing == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var sk RuntimeSkill
	if err := c.ShouldBindJSON(&sk); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	sk.ID = id
	if err := e.runtimeSkillEngine.store.Update(&sk); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, sk)
}

func (e *ChatEngine) DeleteRuntimeSkill(c *gin.Context) {
	id := c.Param("id")
	if err := e.runtimeSkillEngine.store.Delete(id); err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (e *ChatEngine) PinRuntimeSkill(c *gin.Context) {
	id := c.Param("id")
	skill := e.runtimeSkillEngine.store.Get(id)
	if skill == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	skill.State = "pinned"
	skill.UpdatedAt = time.Now()
	e.runtimeSkillEngine.store.Update(skill)
	c.JSON(200, skill)
}

func (e *ChatEngine) ArchiveRuntimeSkill(c *gin.Context) {
	id := c.Param("id")
	skill := e.runtimeSkillEngine.store.Get(id)
	if skill == nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	skill.State = "archived"
	skill.UpdatedAt = time.Now()
	e.runtimeSkillEngine.store.Update(skill)
	c.JSON(200, skill)
}

func (e *ChatEngine) SkillStats(c *gin.Context) {
	skills := e.runtimeSkillEngine.store.List()
	stats := map[string]interface{}{
		"total":     len(skills),
		"active":    0,
		"stale":     0,
		"archived":  0,
		"pinned":    0,
		"by_origin": map[string]int{"runtime": 0, "agent": 0, "user": 0},
	}
	for _, sk := range skills {
		switch sk.State {
		case "active":
			stats["active"] = stats["active"].(int) + 1
		case "stale":
			stats["stale"] = stats["stale"].(int) + 1
		case "archived":
			stats["archived"] = stats["archived"].(int) + 1
		case "pinned":
			stats["pinned"] = stats["pinned"].(int) + 1
		}
		byOrigin := stats["by_origin"].(map[string]int)
		byOrigin[sk.CreatedBy]++
	}
	c.JSON(200, gin.H{"stats": stats})
}
