package vector

import "testing"

func TestMemoryVector(t *testing.T) {
	v := NewMemory()

	// Create collection
	if err := v.CreateCollection("test", 3, "cosine"); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Upsert
	docs := []Document{
		{ID: "1", Content: "hello world", Vector: []float64{1, 0, 0}},
		{ID: "2", Content: "foo bar", Vector: []float64{0, 1, 0}},
		{ID: "3", Content: "hello foo", Vector: []float64{0.7, 0.7, 0}},
	}
	if err := v.Upsert("test", docs); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Search
	results, err := v.Search("test", []float64{1, 0, 0}, SearchOpts{TopK: 2})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected doc 1 first, got %s", results[0].ID)
	}
}

func TestMemorySearchByText(t *testing.T) {
	v := NewMemory()
	v.CreateCollection("docs", 3, "cosine")
	v.Upsert("docs", []Document{
		{ID: "1", Content: "Go is a programming language"},
		{ID: "2", Content: "Python is also a language"},
	})

	results, _ := v.SearchByText("docs", "Go", SearchOpts{TopK: 5})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestMemoryDelete(t *testing.T) {
	v := NewMemory()
	v.CreateCollection("c", 3, "cosine")
	v.Upsert("c", []Document{
		{ID: "1", Content: "a", Vector: []float64{1, 0, 0}},
		{ID: "2", Content: "b", Vector: []float64{0, 1, 0}},
	})

	v.Delete("c", []string{"1"})
	results, _ := v.Search("c", []float64{1, 0, 0}, SearchOpts{TopK: 10})
	if len(results) != 1 {
		t.Fatalf("expected 1 after delete, got %d", len(results))
	}
}

func TestMemoryListCollections(t *testing.T) {
	v := NewMemory()
	v.CreateCollection("a", 3, "cosine")
	v.CreateCollection("b", 3, "cosine")

	cols, _ := v.ListCollections()
	if len(cols) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(cols))
	}
}
