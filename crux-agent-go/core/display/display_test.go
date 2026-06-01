package display

import (
	"testing"
)

func TestSpinner(t *testing.T) {
	s := NewSpinner(nil, nil)
	s.Start("testing")
	s.Stop()
}

func TestSpinnerCustomFrames(t *testing.T) {
	s := NewSpinner([]string{"-", "\\", "|", "/"}, []string{"working"})
	s.Start("test")
	s.Stop()
}

func TestSpinnerUpdateMessage(t *testing.T) {
	s := NewSpinner(nil, nil)
	s.Start("initial")
	s.UpdateMessage("updated")
	s.Stop()
}

func TestActivityFeed(t *testing.T) {
	f := NewActivityFeed(5, "┊")
	f.Add("entry 1")
	f.Add("entry 2")
	if len(f.Entries()) != 2 {
		t.Fatalf("expected 2, got %d", len(f.Entries()))
	}
}

func TestActivityFeedMaxLen(t *testing.T) {
	f := NewActivityFeed(3, "┊")
	f.Add("a")
	f.Add("b")
	f.Add("c")
	f.Add("d")
	if len(f.Entries()) != 3 {
		t.Fatalf("expected 3, got %d", len(f.Entries()))
	}
}

func TestActivityFeedClear(t *testing.T) {
	f := NewActivityFeed(5, "┊")
	f.Add("entry")
	f.Clear()
	if len(f.Entries()) != 0 {
		t.Fatal("expected empty")
	}
}

func TestActivityFeedToolCall(t *testing.T) {
	f := NewActivityFeed(5, "┊")
	f.AddToolCall("exec", "echo hello")
	if len(f.Entries()) != 1 {
		t.Fatal("expected 1 entry")
	}
}

func TestActivityFeedToolResult(t *testing.T) {
	f := NewActivityFeed(5, "┊")
	f.AddToolResult("exec", "hello\nworld")
	if len(f.Entries()) != 1 {
		t.Fatal("expected 1 entry")
	}
}

func TestActivityFeedError(t *testing.T) {
	f := NewActivityFeed(5, "┊")
	f.AddError("something went wrong")
	if len(f.Entries()) != 1 {
		t.Fatal("expected 1 entry")
	}
}

func TestActivityFeedSuccess(t *testing.T) {
	f := NewActivityFeed(5, "┊")
	f.AddSuccess("done")
	if len(f.Entries()) != 1 {
		t.Fatal("expected 1 entry")
	}
}

func TestProgressBar(t *testing.T) {
	bar := ProgressBar(50, 100, 20)
	if bar == "" {
		t.Fatal("expected non-empty")
	}
	bar0 := ProgressBar(0, 100, 20)
	if bar0 == "" {
		t.Fatal("expected non-empty")
	}
	bar100 := ProgressBar(100, 100, 20)
	if bar100 == "" {
		t.Fatal("expected non-empty")
	}
	barZero := ProgressBar(0, 0, 20)
	if barZero != "" {
		t.Fatal("expected empty for zero total")
	}
}
