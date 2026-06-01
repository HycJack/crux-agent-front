// Package environment provides tool execution environment abstraction.
package environment

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

type Environment interface {
	Name() string
	Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Close() error
}

type ExecOpts struct {
	WorkDir string
	Timeout time.Duration
	Env     []string
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// LocalEnv executes commands locally.
type LocalEnv struct{}

func NewLocal() *LocalEnv { return &LocalEnv{} }
func (e *LocalEnv) Name() string { return "local" }

func (e *LocalEnv) Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error) {
	start := time.Now()
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}

	out, err := cmd.CombinedOutput()
	result := ExecResult{
		Stdout:   string(out),
		Duration: time.Since(start),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		result.Stderr = err.Error()
	}
	return result, nil
}

func (e *LocalEnv) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (e *LocalEnv) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func (e *LocalEnv) Close() error { return nil }

// DockerEnv executes commands in a Docker container.
type DockerEnv struct {
	Container string
	Image     string
}

func NewDocker(image string) *DockerEnv {
	return &DockerEnv{Image: image}
}

func (e *DockerEnv) Name() string { return "docker" }

func (e *DockerEnv) Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error) {
	start := time.Now()
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if e.Container == "" {
		if err := e.startContainer(ctx); err != nil {
			return ExecResult{}, fmt.Errorf("start container: %w", err)
		}
	}

	args := []string{"exec", e.Container, "bash", "-c", command}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if opts.WorkDir != "" {
		cmd.Env = append(cmd.Env, "WORK_DIR="+opts.WorkDir)
	}

	out, err := cmd.CombinedOutput()
	result := ExecResult{
		Stdout:   string(out),
		Duration: time.Since(start),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Stderr = err.Error()
	}
	return result, nil
}

func (e *DockerEnv) startContainer(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "run", "-d", "--rm", e.Image, "sleep", "infinity")
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	e.Container = string(bytes.TrimSpace(out))
	return nil
}

func (e *DockerEnv) ReadFile(path string) ([]byte, error) {
	cmd := exec.Command("docker", "exec", e.Container, "cat", path)
	return cmd.Output()
}

func (e *DockerEnv) WriteFile(path string, data []byte) error {
	cmd := exec.Command("docker", "exec", "-i", e.Container, "bash", "-c", fmt.Sprintf("cat > %s", path))
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func (e *DockerEnv) Close() error {
	if e.Container != "" {
		exec.Command("docker", "kill", e.Container).Run()
	}
	return nil
}

// SSHEnv executes commands on a remote host via SSH.
type SSHEnv struct {
	Host    string
	User    string
	KeyPath string
}

func NewSSH(host, user, keyPath string) *SSHEnv {
	return &SSHEnv{Host: host, User: user, KeyPath: keyPath}
}

func (e *SSHEnv) Name() string { return "ssh" }

func (e *SSHEnv) Exec(ctx context.Context, command string, opts ExecOpts) (ExecResult, error) {
	start := time.Now()
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"-o", "StrictHostKeyChecking=no"}
	if e.KeyPath != "" {
		args = append(args, "-i", e.KeyPath)
	}
	args = append(args, fmt.Sprintf("%s@%s", e.User, e.Host), command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	out, err := cmd.CombinedOutput()
	result := ExecResult{
		Stdout:   string(out),
		Duration: time.Since(start),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Stderr = err.Error()
	}
	return result, nil
}

func (e *SSHEnv) ReadFile(path string) ([]byte, error) {
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", e.User, e.Host), fmt.Sprintf("cat %s", path))
	return cmd.Output()
}

func (e *SSHEnv) WriteFile(path string, data []byte) error {
	cmd := exec.Command("ssh", fmt.Sprintf("%s@%s", e.User, e.Host), fmt.Sprintf("cat > %s", path))
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func (e *SSHEnv) Close() error { return nil }

var (
	envMu       sync.RWMutex
	envRegistry = map[string]func(config map[string]any) Environment{}
)

func RegisterEnv(name string, factory func(config map[string]any) Environment) {
	envMu.Lock()
	defer envMu.Unlock()
	envRegistry[name] = factory
}

func CreateEnv(name string, config map[string]any) (Environment, error) {
	envMu.RLock()
	defer envMu.RUnlock()
	factory, ok := envRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown environment: %s", name)
	}
	return factory(config), nil
}

func init() {
	RegisterEnv("local", func(config map[string]any) Environment { return NewLocal() })
	RegisterEnv("docker", func(config map[string]any) Environment {
		image, _ := config["image"].(string)
		if image == "" {
			image = "ubuntu:latest"
		}
		return NewDocker(image)
	})
	RegisterEnv("ssh", func(config map[string]any) Environment {
		host, _ := config["host"].(string)
		user, _ := config["user"].(string)
		keyPath, _ := config["key_path"].(string)
		return NewSSH(host, user, keyPath)
	})
}
