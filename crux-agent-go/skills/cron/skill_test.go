package cron

import (
	"testing"
)

func TestSchedulerAdd(t *testing.T) {
	s := NewScheduler()
	job := s.Add("j1", "test", "30m", "do something")

	if job.ID != "j1" {
		t.Errorf("id: %s", job.ID)
	}
	if !job.Enabled {
		t.Error("should be enabled")
	}
}

func TestSchedulerList(t *testing.T) {
	s := NewScheduler()
	s.Add("j1", "a", "30m", "task1")
	s.Add("j2", "b", "1h", "task2")

	jobs := s.List()
	if len(jobs) != 2 {
		t.Fatalf("expected 2, got %d", len(jobs))
	}
}

func TestSchedulerRemove(t *testing.T) {
	s := NewScheduler()
	s.Add("j1", "a", "30m", "task")
	s.Remove("j1")

	if len(s.List()) != 0 {
		t.Error("should be empty after remove")
	}
}

func TestSchedulerDisableEnable(t *testing.T) {
	s := NewScheduler()
	s.Add("j1", "a", "30m", "task")

	s.Disable("j1")
	pending := s.Pending()
	if len(pending) != 0 {
		t.Error("disabled job should not be pending")
	}

	s.Enable("j1")
	// Note: Pending depends on time, so we can't reliably test it here
}

func TestCronSkillTools(t *testing.T) {
	s := NewSkill()
	caps := s.Capabilities()
	if len(caps.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(caps.Tools))
	}
}
