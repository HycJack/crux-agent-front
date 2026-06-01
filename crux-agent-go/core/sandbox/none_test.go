package sandbox

import "testing"

func TestNoneAllowsEverything(t *testing.T) {
	n := NewNone()

	if err := n.CheckRead("/any/path"); err != nil {
		t.Errorf("CheckRead should allow: %v", err)
	}
	if err := n.CheckWrite("/any/path"); err != nil {
		t.Errorf("CheckWrite should allow: %v", err)
	}
	if err := n.CheckExec("rm -rf /"); err != nil {
		t.Errorf("CheckExec should allow: %v", err)
	}
	if err := n.CheckNetwork("http://evil.com"); err != nil {
		t.Errorf("CheckNetwork should allow: %v", err)
	}
	if env := n.Env(); env != nil {
		t.Errorf("Env should be nil, got %v", env)
	}
}
