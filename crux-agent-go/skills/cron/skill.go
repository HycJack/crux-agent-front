package cron

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hermes-go/core/types"
)

// Job represents a scheduled task.
type Job struct {
	ID        string
	Name      string
	Schedule  string    // cron expression or interval like "30m", "1h"
	Task      string    // prompt to execute
	LastRun   time.Time
	NextRun   time.Time
	Enabled   bool
	RunCount  int
}

// Scheduler manages scheduled tasks.
type Scheduler struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		jobs: make(map[string]*Job),
	}
}

// Add creates a new scheduled job.
func (s *Scheduler) Add(id, name, schedule, task string) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := &Job{
		ID:       id,
		Name:     name,
		Schedule: schedule,
		Task:     task,
		Enabled:  true,
		NextRun:  s.parseNextRun(schedule),
	}
	s.jobs[id] = job
	return job
}

// Remove deletes a job.
func (s *Scheduler) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

// Enable enables a job.
func (s *Scheduler) Enable(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Enabled = true
	}
}

// Disable disables a job.
func (s *Scheduler) Disable(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Enabled = false
	}
}

// Pending returns jobs that are due to run.
func (s *Scheduler) Pending() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var pending []*Job
	for _, job := range s.jobs {
		if job.Enabled && now.After(job.NextRun) {
			pending = append(pending, job)
		}
	}
	return pending
}

// MarkRun updates a job after execution.
func (s *Scheduler) MarkRun(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.LastRun = time.Now()
		job.RunCount++
		job.NextRun = s.parseNextRun(job.Schedule)
	}
}

// List returns all jobs.
func (s *Scheduler) List() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	var jobs []*Job
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

func (s *Scheduler) parseNextRun(schedule string) time.Time {
	// Parse simple intervals: "30m", "1h", "2h30m"
	if d, err := time.ParseDuration(schedule); err == nil {
		return time.Now().Add(d)
	}
	// Default: 1 hour
	return time.Now().Add(time.Hour)
}

// Skill exposes cron as tools.
type Skill struct {
	scheduler *Scheduler
}

func NewSkill() *Skill {
	return &Skill{scheduler: NewScheduler()}
}

func (s *Skill) Name() string { return "cron" }

func (s *Skill) Capabilities() struct{ Tools []types.ToolSchema } {
	return struct{ Tools []types.ToolSchema }{
		Tools: []types.ToolSchema{
			{
				Name:        "cron_add",
				Description: "Add a scheduled job that runs periodically.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string", "description": "Job name"},
						"schedule": map[string]any{"type": "string", "description": "Interval like 30m, 1h, 2h"},
						"task":     map[string]any{"type": "string", "description": "Task prompt to execute"},
					},
					"required": []string{"name", "schedule", "task"},
				},
			},
			{
				Name:        "cron_list",
				Description: "List all scheduled jobs.",
			},
			{
				Name:        "cron_remove",
				Description: "Remove a scheduled job.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "string", "description": "Job ID"},
					},
					"required": []string{"id"},
				},
			},
		},
	}
}

func (s *Skill) Handle(call types.ToolCall) types.ToolResult {
	var args map[string]any
	json.Unmarshal([]byte(call.Function.Arguments), &args)

	var content string
	var err error

	switch call.Function.Name {
	case "cron_add":
		name, _ := args["name"].(string)
		schedule, _ := args["schedule"].(string)
		task, _ := args["task"].(string)
		job := s.scheduler.Add(fmt.Sprintf("job-%d", time.Now().UnixNano()), name, schedule, task)
		content = fmt.Sprintf("Added job %s (%s), next run: %s", job.ID, job.Name, job.NextRun.Format("15:04:05"))

	case "cron_list":
		jobs := s.scheduler.List()
		if len(jobs) == 0 {
			content = "No scheduled jobs."
		} else {
			for _, j := range jobs {
				content += fmt.Sprintf("[%s] %s (%s) runs=%d enabled=%v\n", j.ID, j.Name, j.Schedule, j.RunCount, j.Enabled)
			}
		}

	case "cron_remove":
		id, _ := args["id"].(string)
		s.scheduler.Remove(id)
		content = fmt.Sprintf("Removed job %s", id)

	default:
		err = fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	result := types.ToolResult{ToolCallID: call.ID}
	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.IsError = true
	} else {
		result.Content = content
	}
	return result
}
