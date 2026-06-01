package sandbox

import (
	"fmt"
	"os/exec"
	"strings"
)

// Process is a process-level sandbox with restricted access.
type Process struct {
	readPaths    []string
	writePaths   []string
	allowedCmds  []string
	blockedCmds  []string
	envWhitelist []string
	timeout      int // seconds
}

type ProcessConfig struct {
	ReadOnly    []string
	ReadWrite   []string
	AllowCmds   []string
	BlockCmds   []string
	EnvWhitelist []string
	Timeout     int
}

func NewProcess(cfg ProcessConfig) *Process {
	if cfg.Timeout == 0 {
		cfg.Timeout = 300
	}
	return &Process{
		readPaths:    cfg.ReadOnly,
		writePaths:   cfg.ReadWrite,
		allowedCmds:  cfg.AllowCmds,
		blockedCmds:  cfg.BlockCmds,
		envWhitelist: cfg.EnvWhitelist,
		timeout:      cfg.Timeout,
	}
}

func (p *Process) CheckRead(path string) error {
	if len(p.readPaths) == 0 && len(p.writePaths) == 0 {
		return nil // no restrictions
	}
	for _, allowed := range append(p.readPaths, p.writePaths...) {
		if strings.HasPrefix(path, allowed) {
			return nil
		}
	}
	return fmt.Errorf("read access denied: %s", path)
}

func (p *Process) CheckWrite(path string) error {
	if len(p.writePaths) == 0 {
		return nil
	}
	for _, allowed := range p.writePaths {
		if strings.HasPrefix(path, allowed) {
			return nil
		}
	}
	return fmt.Errorf("write access denied: %s", path)
}

func (p *Process) CheckExec(cmd string) error {
	// Check blocked commands first
	for _, blocked := range p.blockedCmds {
		if strings.Contains(cmd, blocked) {
			return fmt.Errorf("command blocked: %s", blocked)
		}
	}
	// If allow list is set, command must be in it
	if len(p.allowedCmds) > 0 {
		firstWord := strings.Fields(cmd)[0]
		for _, allowed := range p.allowedCmds {
			if firstWord == allowed {
				return nil
			}
		}
		return fmt.Errorf("command not allowed: %s", firstWord)
	}
	return nil
}

func (p *Process) CheckNetwork(url string) error {
	// Process sandbox doesn't restrict network by default
	return nil
}

func (p *Process) Env() []string {
	if len(p.envWhitelist) == 0 {
		return nil
	}
	var env []string
	for _, key := range p.envWhitelist {
		val := getEnv(key)
		if val != "" {
			env = append(env, key+"="+val)
		}
	}
	return env
}

// Exec runs a command within the sandbox.
func (p *Process) Exec(cmd string) (string, error) {
	if err := p.CheckExec(cmd); err != nil {
		return "", err
	}
	out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	return string(out), err
}

func getEnv(key string) string {
	out, err := exec.Command("bash", "-c", "echo $"+key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
