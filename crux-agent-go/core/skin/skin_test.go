package skin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSkin(t *testing.T) {
	if Default.Name != "default" {
		t.Fatalf("expected default, got %s", Default.Name)
	}
	if Default.Banner.Text != "HERMES" {
		t.Fatal("expected HERMES")
	}
}

func TestRenderBanner(t *testing.T) {
	banner := Default.RenderBanner()
	if banner == "" {
		t.Fatal("expected non-empty")
	}
}

func TestNextSpinner(t *testing.T) {
	s := Default
	f0 := s.NextSpinner(0)
	f1 := s.NextSpinner(1)
	if f0 == f1 {
		t.Fatal("expected different frames")
	}
	// Wrap around
	fLast := s.NextSpinner(len(s.Spinner.Faces))
	if fLast != f0 {
		t.Fatal("expected wrap around")
	}
}

func TestRandomVerb(t *testing.T) {
	s := Default
	v0 := s.RandomVerb(0)
	if v0 == "" {
		t.Fatal("expected non-empty verb")
	}
}

func TestFormatTool(t *testing.T) {
	s := Default
	result := s.FormatTool("exec", "echo hello")
	if result == "" {
		t.Fatal("expected non-empty")
	}
}

func TestLoadSkin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte(`name: test
banner:
  text: TEST
  color: "#ff0000"
spinner:
  faces: ["⠋"]
  verbs: ["thinking"]`), 0644)

	skin, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if skin.Name != "test" {
		t.Fatalf("expected test, got %s", skin.Name)
	}
	if skin.Banner.Text != "TEST" {
		t.Fatal("expected TEST")
	}
}

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "cyber.yaml"), []byte(`name: cyber`), 0644)

	skin, err := LoadFromDir(dir, "cyber")
	if err != nil {
		t.Fatal(err)
	}
	if skin.Name != "cyber" {
		t.Fatal("expected cyber")
	}
}

func TestLoadFromDirNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFromDir(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListSkins(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(`name: a`), 0644)
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(`name: b`), 0644)

	skins := ListSkins(dir)
	if len(skins) != 2 {
		t.Fatalf("expected 2, got %d", len(skins))
	}
}

func TestEmptySpinnerFrames(t *testing.T) {
	s := Skin{Spinner: Spinner{Faces: nil, Verbs: nil}}
	if s.NextSpinner(0) != "⠋" {
		t.Fatal("expected default frame")
	}
	if s.RandomVerb(0) != "thinking" {
		t.Fatal("expected default verb")
	}
}
