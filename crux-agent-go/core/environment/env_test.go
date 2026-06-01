package environment

import (
	"context"
	"testing"
	"time"
)

func TestLocalExec(t *testing.T) {
	env := NewLocal()
	result, err := env.Exec(context.Background(), "echo hello", ExecOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected 0, got %d", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Fatalf("expected hello, got %s", result.Stdout)
	}
}

func TestLocalExecTimeout(t *testing.T) {
	env := NewLocal()
	result, err := env.Exec(context.Background(), "sleep 10", ExecOpts{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code for timeout")
	}
}

func TestLocalExecWorkDir(t *testing.T) {
	env := NewLocal()
	result, err := env.Exec(context.Background(), "pwd", ExecOpts{WorkDir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "/tmp\n" {
		t.Fatalf("expected /tmp, got %s", result.Stdout)
	}
}

func TestLocalExecFailingCommand(t *testing.T) {
	env := NewLocal()
	result, _ := env.Exec(context.Background(), "false", ExecOpts{})
	if result.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestLocalReadFile(t *testing.T) {
	env := NewLocal()
	// /etc/hostname should exist on most Linux systems
	_, err := env.ReadFile("/etc/hostname")
	if err != nil {
		t.Skip("skipping: /etc/hostname not found")
	}
}

func TestLocalName(t *testing.T) {
	env := NewLocal()
	if env.Name() != "local" {
		t.Fatalf("expected local, got %s", env.Name())
	}
}

func TestRegistryCreate(t *testing.T) {
	env, err := CreateEnv("local", nil)
	if err != nil {
		t.Fatal(err)
	}
	if env.Name() != "local" {
		t.Fatalf("expected local, got %s", env.Name())
	}
}

func TestRegistryUnknown(t *testing.T) {
	_, err := CreateEnv("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDockerName(t *testing.T) {
	env := NewDocker("ubuntu:latest")
	if env.Name() != "docker" {
		t.Fatalf("expected docker, got %s", env.Name())
	}
}

func TestSSHName(t *testing.T) {
	env := NewSSH("localhost", "root", "")
	if env.Name() != "ssh" {
		t.Fatalf("expected ssh, got %s", env.Name())
	}
}
