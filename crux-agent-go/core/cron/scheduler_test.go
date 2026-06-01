package cron

import (
	"testing"
	"time"
)

func TestSchedulerAdd(t *testing.T) {
	store := NewFileStore(t.TempDir() + "/jobs.json")
	s := NewScheduler(store, func(job *Job) error { return nil })

	job := &Job{Name: "test", Schedule: "1h", Prompt: "do something"}
	if err := s.Add(job); err != nil {
		t.Fatal(err)
	}

	jobs := s.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1, got %d", len(jobs))
	}
	if jobs[0].Name != "test" {
		t.Fatalf("expected test, got %s", jobs[0].Name)
	}
}

func TestSchedulerRemove(t *testing.T) {
	store := NewFileStore(t.TempDir() + "/jobs.json")
	s := NewScheduler(store, func(job *Job) error { return nil })

	job := &Job{Name: "test", Schedule: "1h", Prompt: "do something"}
	s.Add(job)
	s.Remove(job.ID)

	jobs := s.List()
	if len(jobs) != 0 {
		t.Fatalf("expected 0, got %d", len(jobs))
	}
}

func TestSchedulerEnableDisable(t *testing.T) {
	store := NewFileStore(t.TempDir() + "/jobs.json")
	s := NewScheduler(store, func(job *Job) error { return nil })

	job := &Job{Name: "test", Schedule: "1h", Prompt: "do something"}
	s.Add(job)

	s.Disable(job.ID)
	jobs := s.List()
	if jobs[0].Enabled {
		t.Fatal("expected disabled")
	}

	s.Enable(job.ID)
	jobs = s.List()
	if !jobs[0].Enabled {
		t.Fatal("expected enabled")
	}
}

func TestSchedulerPersistence(t *testing.T) {
	path := t.TempDir() + "/jobs.json"
	store := NewFileStore(path)
	s1 := NewScheduler(store, func(job *Job) error { return nil })
	s1.Add(&Job{Name: "test", Schedule: "1h", Prompt: "do something"})

	store2 := NewFileStore(path)
	s2 := NewScheduler(store2, func(job *Job) error { return nil })
	s2.Start()
	defer s2.Stop()

	// Give it a moment to load
	time.Sleep(10 * time.Millisecond)
	jobs := s2.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 persisted job, got %d", len(jobs))
	}
}
