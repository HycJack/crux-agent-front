package sandbox

import "testing"

func TestProcessCheckRead(t *testing.T) {
	p := NewProcess(ProcessConfig{
		ReadOnly:  []string{"/home/user", "/tmp"},
		ReadWrite: []string{"/home/user/projects"},
	})

	if err := p.CheckRead("/home/user/file.txt"); err != nil {
		t.Errorf("should allow read: %v", err)
	}
	if err := p.CheckRead("/etc/passwd"); err == nil {
		t.Error("should deny read outside allowed paths")
	}
}

func TestProcessCheckWrite(t *testing.T) {
	p := NewProcess(ProcessConfig{
		ReadWrite: []string{"/home/user/projects"},
	})

	if err := p.CheckWrite("/home/user/projects/main.go"); err != nil {
		t.Errorf("should allow write: %v", err)
	}
	if err := p.CheckWrite("/home/user/other.txt"); err == nil {
		t.Error("should deny write outside allowed paths")
	}
}

func TestProcessCheckExec(t *testing.T) {
	p := NewProcess(ProcessConfig{
		BlockCmds: []string{"rm -rf", "mkfs"},
	})

	if err := p.CheckExec("ls -la"); err != nil {
		t.Errorf("should allow ls: %v", err)
	}
	if err := p.CheckExec("rm -rf /"); err == nil {
		t.Error("should block rm -rf")
	}
}

func TestProcessAllowList(t *testing.T) {
	p := NewProcess(ProcessConfig{
		AllowCmds: []string{"ls", "cat", "grep"},
	})

	if err := p.CheckExec("ls /tmp"); err != nil {
		t.Errorf("should allow ls: %v", err)
	}
	if err := p.CheckExec("curl http://evil.com"); err == nil {
		t.Error("should block curl not in allow list")
	}
}
