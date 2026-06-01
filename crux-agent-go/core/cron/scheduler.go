package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Job struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Schedule string    `json:"schedule"`
	Prompt   string    `json:"prompt"`
	Skills   []string  `json:"skills,omitempty"`
	Channel  string    `json:"channel,omitempty"`
	Enabled  bool      `json:"enabled"`
	LastRun  time.Time `json:"last_run,omitempty"`
	NextRun  time.Time `json:"next_run,omitempty"`
	RunCount int       `json:"run_count"`
}

type Handler func(job *Job) error

type Scheduler struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	store   Store
	handler Handler
	stopCh  chan struct{}
	running bool
}

type Store interface {
	Save(jobs []*Job) error
	Load() ([]*Job, error)
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Save(jobs []*Job) error {
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(s.path), 0755)
	return os.WriteFile(s.path, data, 0644)
}

func (s *FileStore) Load() ([]*Job, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func NewScheduler(store Store, handler Handler) *Scheduler {
	return &Scheduler{
		jobs:    make(map[string]*Job),
		store:   store,
		handler: handler,
		stopCh:  make(chan struct{}),
	}
}

func (s *Scheduler) Add(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	job.NextRun = s.parseNextRun(job.Schedule)
	job.Enabled = true
	s.jobs[job.ID] = job
	return s.persist()
}

func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	return s.persist()
}

func (s *Scheduler) Enable(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Enabled = true
		return s.persist()
	}
	return fmt.Errorf("job %s not found", id)
}

func (s *Scheduler) Disable(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		job.Enabled = false
		return s.persist()
	}
	return fmt.Errorf("job %s not found", id)
}

func (s *Scheduler) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var jobs []*Job
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	if s.store != nil {
		jobs, err := s.store.Load()
		if err == nil {
			s.mu.Lock()
			for _, j := range jobs {
				s.jobs[j.ID] = j
			}
			s.mu.Unlock()
		}
	}

	go s.loop()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.checkAndRun(now)
		}
	}
}

// FIX: collect jobs to run under lock, then call handler outside lock.
func (s *Scheduler) checkAndRun(now time.Time) {
	s.mu.Lock()
	var toRun []*Job
	for _, job := range s.jobs {
		if job.Enabled && !job.NextRun.IsZero() && now.After(job.NextRun) {
			toRun = append(toRun, job)
		}
	}
	s.mu.Unlock()

	// Call handler outside lock to avoid deadlock
	for _, job := range toRun {
		if s.handler != nil {
			s.handler(job)
		}
		s.mu.Lock()
		job.LastRun = now
		job.RunCount++
		job.NextRun = s.parseNextRun(job.Schedule)
		s.mu.Unlock()
	}

	if len(toRun) > 0 {
		s.mu.Lock()
		s.persist()
		s.mu.Unlock()
	}
}

func (s *Scheduler) parseNextRun(schedule string) time.Time {
	now := time.Now()
	switch schedule {
	case "1m":
		return now.Add(1 * time.Minute)
	case "5m":
		return now.Add(5 * time.Minute)
	case "15m":
		return now.Add(15 * time.Minute)
	case "30m":
		return now.Add(30 * time.Minute)
	case "1h":
		return now.Add(1 * time.Hour)
	case "2h":
		return now.Add(2 * time.Hour)
	case "6h":
		return now.Add(6 * time.Hour)
	case "12h":
		return now.Add(12 * time.Hour)
	case "24h", "daily":
		return now.Add(24 * time.Hour)
	default:
		return now.Add(1 * time.Hour)
	}
}

func (s *Scheduler) persist() error {
	if s.store == nil {
		return nil
	}
	var jobs []*Job
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return s.store.Save(jobs)
}
