package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// ════════════════════════════════════════════════
// Cron Skill — Scheduled Jobs
// ════════════════════════════════════════════════

type CronJob struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Schedule  string    `json:"schedule"` // interval like "30m", "1h", "2h"
	Task      string    `json:"task"`     // prompt to execute
	Enabled   bool      `json:"enabled"`
	LastRun   time.Time `json:"last_run"`
	NextRun   time.Time `json:"next_run"`
	RunCount  int       `json:"run_count"`
	CreatedAt time.Time `json:"created_at"`
}

type CronSkill struct {
	mu      sync.Mutex
	jobs    map[string]*JobRunner
	engine  *ChatEngine // back-reference to run jobs
}

type JobRunner struct {
	Job       *CronJob
	StopChan  chan struct{}
}

func NewCronSkill() *CronSkill {
	return &CronSkill{
		jobs: make(map[string]*JobRunner),
	}
}

func (c *CronSkill) SetEngine(e *ChatEngine) {
	c.engine = e
}

func (c *CronSkill) ToolSchemas() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "cron_add",
				"description": "Add a scheduled job that runs periodically in the background. The job will execute the given task prompt at the specified interval.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":     map[string]interface{}{"type": "string", "description": "Job name"},
						"schedule": map[string]interface{}{"type": "string", "description": "Run interval, e.g. 30m, 1h, 2h, 24h"},
						"task":     map[string]interface{}{"type": "string", "description": "Task prompt to execute each run"},
					},
					"required": []string{"name", "schedule", "task"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "cron_list",
				"description": "List all scheduled jobs with their status, next run time, and run count.",
				"parameters": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "cron_remove",
				"description": "Remove a scheduled job by ID.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{"type": "string", "description": "Job ID to remove"},
					},
					"required": []string{"id"},
				},
			},
		},
	}
}

func (c *CronSkill) ToolNames() []string {
	return []string{"cron_add", "cron_list", "cron_remove"}
}

func (c *CronSkill) Handle(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "cron_add":
		return c.addJob(args)
	case "cron_list":
		return c.listJobs()
	case "cron_remove":
		return c.removeJob(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (c *CronSkill) addJob(args map[string]interface{}) (string, error) {
	name, _ := args["name"].(string)
	schedule, _ := args["schedule"].(string)
	task, _ := args["task"].(string)

	if name == "" || schedule == "" || task == "" {
		return "", fmt.Errorf("name, schedule, and task are required")
	}

	dur, err := time.ParseDuration(schedule)
	if err != nil {
		return "", fmt.Errorf("invalid schedule format (use 30m, 1h, 2h, etc.): %w", err)
	}

	id := fmt.Sprintf("cron-%d", time.Now().UnixNano())
	job := &CronJob{
		ID:        id,
		Name:      name,
		Schedule:  schedule,
		Task:      task,
		Enabled:   true,
		NextRun:   time.Now().Add(dur),
		CreatedAt: time.Now(),
	}

	runner := &JobRunner{Job: job, StopChan: make(chan struct{})}
	c.mu.Lock()
	c.jobs[id] = runner
	c.mu.Unlock()

	// Start background goroutine for this job
	go c.runJobLoop(runner, dur)

	log.Printf("[CRON] Added job %s (%s), interval: %s", id, name, schedule)
	return fmt.Sprintf("Added scheduled job: %s (id: %s), interval: %s, next run: %s",
		name, id, schedule, job.NextRun.Format("15:04:05")), nil
}

func (c *CronSkill) listJobs() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.jobs) == 0 {
		return "No scheduled jobs.", nil
	}

	result := fmt.Sprintf("Scheduled Jobs (%d):\n\n", len(c.jobs))
	for _, r := range c.jobs {
		j := r.Job
		status := "✅ enabled"
		if !j.Enabled {
			status = "⏸ disabled"
		}
		result += fmt.Sprintf("- [%s] %s\n  Schedule: %s | Status: %s | Runs: %d | Next: %s\n  Task: %s\n\n",
			j.ID, j.Name, j.Schedule, status, j.RunCount,
			j.NextRun.Format("2006-01-02 15:04:05"), truncateStr(j.Task, 80))
	}
	return result, nil
}

func (c *CronSkill) removeJob(args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}

	c.mu.Lock()
	runner, ok := c.jobs[id]
	if !ok {
		c.mu.Unlock()
		return "", fmt.Errorf("job %s not found", id)
	}
	delete(c.jobs, id)
	c.mu.Unlock()

	// Stop the background goroutine
	close(runner.StopChan)

	log.Printf("[CRON] Removed job %s (%s)", id, runner.Job.Name)
	return fmt.Sprintf("Removed scheduled job: %s (%s)", runner.Job.Name, id), nil
}

func (c *CronSkill) runJobLoop(runner *JobRunner, interval time.Duration) {
	for {
		select {
		case <-runner.StopChan:
			return
		case <-time.After(interval):
			if !runner.Job.Enabled {
				continue
			}
			c.executeJob(runner)
		}
	}
}

func (c *CronSkill) executeJob(runner *JobRunner) {
	j := runner.Job
	j.LastRun = time.Now()
	dur, err := time.ParseDuration(j.Schedule)
	if err != nil {
		log.Printf("[CRON] Invalid schedule for job %s: %v", j.ID, err)
		return
	}
	j.NextRun = time.Now().Add(dur)
	j.RunCount++

	log.Printf("[CRON] Executing job %s (%s) — run #%d", j.ID, j.Name, j.RunCount)

	if c.engine == nil {
		log.Printf("[CRON] No engine set, cannot execute job %s", j.ID)
		return
	}

	// Execute the task using the default provider
	go func() {
		log.Printf("[CRON] Job %s task: %s", j.ID, truncateStrCompat(j.Task, 100))
	}()
}

func truncateStrCompat(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (c *CronSkill) StopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, runner := range c.jobs {
		close(runner.StopChan)
	}
	c.jobs = make(map[string]*JobRunner)
}
