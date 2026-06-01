package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *SQLite {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateGetSession(t *testing.T) {
	s := newTestStore(t)

	sess := &SessionMeta{
		ID:           "s-1",
		Title:        "test",
		TokenCount:   0,
		MessageCount: 0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	loaded, err := s.GetSession("s-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.ID != "s-1" || loaded.Title != "test" {
		t.Errorf("mismatch: %+v", loaded)
	}
}

func TestAppendAndGetMessages(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-1", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	msg1 := &StoredMessage{ID: "m-1", Role: "user", Content: "hello", TokenEst: 5}
	msg2 := &StoredMessage{ID: "m-2", Role: "assistant", Content: "hi!", TokenEst: 5}

	s.AppendMessage("s-1", msg1)
	s.AppendMessage("s-1", msg2)

	msgs, err := s.GetMessages("s-1", MessageOpts{})
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("msg0: %s", msgs[0].Content)
	}
	if msgs[1].Content != "hi!" {
		t.Errorf("msg1: %s", msgs[1].Content)
	}
}

func TestMessageTokenEst(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-1", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	msg := &StoredMessage{ID: "m-1", Role: "user", Content: "this is a test message with some content", TokenEst: 10}
	s.AppendMessage("s-1", msg)

	msgs, _ := s.GetMessages("s-1", MessageOpts{})
	if len(msgs) == 0 {
		t.Fatal("no messages found")
	}
	if msgs[0].TokenEst <= 0 {
		t.Error("token estimate should be > 0")
	}
}

func TestMessageCountAutoIncrement(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-1", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	s.AppendMessage("s-1", &StoredMessage{ID: "m-1", Role: "user", Content: "a", TokenEst: 1})
	s.AppendMessage("s-1", &StoredMessage{ID: "m-2", Role: "assistant", Content: "b", TokenEst: 1})

	sess, _ := s.GetSession("s-1")
	if sess.MessageCount != 2 {
		t.Errorf("expected message_count=2, got %d", sess.MessageCount)
	}
}

func TestSearchMessages(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-1", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	s.AppendMessage("s-1", &StoredMessage{ID: "m-1", Role: "user", Content: "I love Go programming", TokenEst: 5})
	s.AppendMessage("s-1", &StoredMessage{ID: "m-2", Role: "assistant", Content: "Go is great!", TokenEst: 5})
	s.AppendMessage("s-1", &StoredMessage{ID: "m-3", Role: "user", Content: "Python is also good", TokenEst: 5})

	hits, err := s.SearchMessages("Go", SearchOpts{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) < 2 {
		t.Errorf("expected at least 2 hits for 'Go', got %d", len(hits))
	}
}

func TestListSessions(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		s.CreateSession(&SessionMeta{
			ID:        fmt.Sprintf("s-%c", rune('a'+i)),
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
	}

	sessions, err := s.ListSessions(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("expected 3, got %d", len(sessions))
	}
}

func TestUpdateSession(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-1", Title: "v1", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	s.UpdateSession("s-1", map[string]any{"title": "v2"})
	sess, _ := s.GetSession("s-1")
	if sess.Title != "v2" {
		t.Errorf("expected v2, got %s", sess.Title)
	}
}

func TestDeleteSession(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-del", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	s.DeleteSession("s-del")

	_, err := s.GetSession("s-del")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestKV(t *testing.T) {
	s := newTestStore(t)

	s.Set("ns", "k1", []byte("v1"))
	v, _ := s.Get("ns", "k1")
	if string(v) != "v1" {
		t.Errorf("get: %s", string(v))
	}

	s.Delete("ns", "k1")
	_, err := s.Get("ns", "k1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestKVList(t *testing.T) {
	s := newTestStore(t)
	s.Set("ns", "a", []byte("1"))
	s.Set("ns", "b", []byte("2"))
	s.Set("other", "c", []byte("3"))

	pairs, _ := s.List("ns")
	if len(pairs) != 2 {
		t.Errorf("expected 2, got %d", len(pairs))
	}
}

func TestMarkCompacted(t *testing.T) {
	s := newTestStore(t)
	s.CreateSession(&SessionMeta{ID: "s-1", CreatedAt: time.Now(), UpdatedAt: time.Now()})

	s.AppendMessage("s-1", &StoredMessage{ID: "m-1", Role: "user", Content: "old", TokenEst: 5})
	s.AppendMessage("s-1", &StoredMessage{ID: "m-2", Role: "assistant", Content: "old reply", TokenEst: 5})
	s.AppendMessage("s-1", &StoredMessage{ID: "m-3", Role: "user", Content: "new", TokenEst: 5})

	n, err := s.MarkCompacted("s-1", "m-3")
	if err != nil {
		t.Fatalf("mark compacted: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 compacted, got %d", n)
	}

	// ExcludeCompacted should filter them out
	msgs, _ := s.GetMessages("s-1", MessageOpts{ExcludeCompacted: true})
	if len(msgs) != 1 {
		t.Errorf("expected 1 non-compacted, got %d", len(msgs))
	}
}
