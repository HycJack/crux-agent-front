package memory

import (
	"testing"
)

func TestStoreSaveAndSearch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	store.Save("memory", "project", "Building Hermes in Go")
	store.Save("user", "name", "Test User")

	results := store.Search("Hermes", "")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Key != "project" {
		t.Fatalf("expected project, got %s", results[0].Key)
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	store.Save("memory", "a", "1")
	store.Save("memory", "b", "2")
	store.Save("user", "c", "3")

	memories := store.List("memory")
	if len(memories) != 2 {
		t.Fatalf("expected 2, got %d", len(memories))
	}
}

func TestStoreDelete(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	store.Save("memory", "temp", "to be deleted")
	if err := store.Delete("temp"); err != nil {
		t.Fatal(err)
	}

	results := store.Search("temp", "")
	if len(results) != 0 {
		t.Fatalf("expected 0, got %d", len(results))
	}
}

func TestStoreGetText(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	store.Save("user", "name", "Alice")
	store.Save("memory", "project", "Hermes")

	text := store.GetText()
	if text == "" {
		t.Fatal("expected non-empty text")
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	store1, _ := NewStore(dir)
	store1.Save("memory", "key1", "value1")

	store2, _ := NewStore(dir)
	results := store2.Search("key1", "")
	if len(results) != 1 {
		t.Fatal("expected memory to persist")
	}
}
