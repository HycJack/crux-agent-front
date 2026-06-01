// Package display provides CLI display utilities.
// Inspired by Hermes Python's agent/display.py: KawaiiSpinner, activity feed.
package display

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Spinner shows an animated spinner with status text.
type Spinner struct {
	frames  []string
	verbs   []string
	active  bool
	mu      sync.Mutex
	stopCh  chan struct{}
	message string
}

// NewSpinner creates a new spinner.
func NewSpinner(frames, verbs []string) *Spinner {
	if len(frames) == 0 {
		frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	}
	if len(verbs) == 0 {
		verbs = []string{"thinking", "processing", "computing"}
	}
	return &Spinner{frames: frames, verbs: verbs, stopCh: make(chan struct{})}
}

// Start starts the spinner with a message.
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.message = message
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go func() {
		frame := 0
		verbIdx := 0
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.mu.Lock()
				verb := s.verbs[verbIdx%len(s.verbs)]
				face := s.frames[frame%len(s.frames)]
				fmt.Printf("\r\033[36m%s\033[0m %s... ", face, verb)
				s.mu.Unlock()
				frame++
				if frame%15 == 0 {
					verbIdx++
				}
			}
		}
	}()
}

// Stop stops the spinner.
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.active = false
	close(s.stopCh)
	fmt.Print("\r\033[K") // Clear line
}

// UpdateMessage updates the spinner message.
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// ActivityFeed shows a scrolling activity feed.
type ActivityFeed struct {
	mu      sync.Mutex
	entries []string
	maxLen  int
	prefix  string
}

// NewActivityFeed creates a new activity feed.
func NewActivityFeed(maxLen int, prefix string) *ActivityFeed {
	if maxLen == 0 {
		maxLen = 10
	}
	if prefix == "" {
		prefix = "┊"
	}
	return &ActivityFeed{maxLen: maxLen, prefix: prefix}
}

// Add adds an entry to the feed.
func (f *ActivityFeed) Add(entry string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
	if len(f.entries) > f.maxLen {
		f.entries = f.entries[1:]
	}
	fmt.Printf("  %s %s\n", f.prefix, entry)
}

// AddToolCall adds a tool call entry.
func (f *ActivityFeed) AddToolCall(tool, args string) {
	truncated := args
	if len(truncated) > 60 {
		truncated = truncated[:60] + "..."
	}
	f.Add(fmt.Sprintf("\033[33m%s\033[0m(%s)", tool, truncated))
}

// AddToolResult adds a tool result entry.
func (f *ActivityFeed) AddToolResult(tool, result string) {
	truncated := result
	if len(truncated) > 80 {
		truncated = truncated[:80] + "..."
	}
	// Replace newlines
	truncated = strings.ReplaceAll(truncated, "\n", " ")
	f.Add(fmt.Sprintf("\033[32m%s\033[0m → %s", tool, truncated))
}

// AddError adds an error entry.
func (f *ActivityFeed) AddError(msg string) {
	f.Add(fmt.Sprintf("\033[31m✗ %s\033[0m", msg))
}

// AddSuccess adds a success entry.
func (f *ActivityFeed) AddSuccess(msg string) {
	f.Add(fmt.Sprintf("\033[32m✓ %s\033[0m", msg))
}

// Clear clears the feed.
func (f *ActivityFeed) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
}

// Entries returns current entries.
func (f *ActivityFeed) Entries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.entries...)
}

// ProgressBar shows a progress bar.
func ProgressBar(current, total int, width int) string {
	if width == 0 {
		width = 30
	}
	if total == 0 {
		return ""
	}
	filled := current * width / total
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	pct := current * 100 / total
	return fmt.Sprintf("[%s] %d%%", bar, pct)
}
