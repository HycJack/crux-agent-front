package result

import (
	"fmt"
	"testing"
)

func TestOk(t *testing.T) {
	r := Ok[int, error](42)
	if !r.Ok {
		t.Fatal("expected ok")
	}
	if v, ok := r.Unwrap(); !ok || v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
}

func TestError(t *testing.T) {
	r := Error[int, error](fmt.Errorf("test"))
	if r.Ok {
		t.Fatal("expected error")
	}
	if _, ok := r.Unwrap(); ok {
		t.Fatal("expected no value")
	}
}

func TestGetOrDefault(t *testing.T) {
	ok := Ok[string, error]("value")
	if ok.GetOrDefault("default") != "value" {
		t.Fatal("expected value")
	}

	err := Error[string, error](fmt.Errorf("test"))
	if err.GetOrDefault("default") != "default" {
		t.Fatal("expected default")
	}
}

func TestMap(t *testing.T) {
	r := Ok[int, error](42)
	mapped := Map(r, func(v int) string { return fmt.Sprintf("%d!", v) })
	if mapped.GetOrDefault("") != "42!" {
		t.Fatal("expected 42!")
	}

	err := Error[int, error](fmt.Errorf("test"))
	mapped2 := Map(err, func(v int) string { return "never" })
	if mapped2.Ok {
		t.Fatal("expected error")
	}
}

func TestMapError(t *testing.T) {
	r := Error[int, string]("oops")
	mapped := MapError(r, func(e string) int { return len(e) })
	if mapped.Ok {
		t.Fatal("expected error")
	}
	if mapped.Err != 4 {
		t.Fatalf("expected 4, got %d", mapped.Err)
	}
}

func TestFlatMap(t *testing.T) {
	r := Ok[int, error](21)
	doubled := FlatMap(r, func(v int) Result[int, error] {
		return Ok[int, error](v * 2)
	})
	if doubled.GetOrDefault(0) != 42 {
		t.Fatal("expected 42")
	}

	err := Error[int, error](fmt.Errorf("test"))
	doubled2 := FlatMap(err, func(v int) Result[int, error] {
		return Ok[int, error](v * 2)
	})
	if doubled2.Ok {
		t.Fatal("expected error")
	}
}

func TestGetOrThrowPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	r := Error[int, error](fmt.Errorf("test"))
	r.GetOrThrow()
}
