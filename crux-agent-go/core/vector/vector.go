package vector

// Vector is the interface for vector storage and retrieval.
type Vector interface {
	Open(config Config) error
	Close() error
	Upsert(collection string, docs []Document) error
	Delete(collection string, ids []string) error
	Search(collection string, query []float64, opts SearchOpts) ([]Result, error)
	SearchByText(collection string, query string, opts SearchOpts) ([]Result, error)
	CreateCollection(name string, dim int, metric string) error
	DeleteCollection(name string) error
	ListCollections() ([]string, error)
}

type Document struct {
	ID       string
	Content  string
	Vector   []float64
	Metadata map[string]any
}

type Result struct {
	ID       string
	Content  string
	Score    float64
	Metadata map[string]any
}

type SearchOpts struct {
	TopK     int
	MinScore float64
	Filter   map[string]any
}

type Config struct {
	Driver  string         // "memory", "qdrant", "pgvector"
	URL     string
	APIKey  string
	Options map[string]any
}

// Memory is an in-memory vector store for development/testing.
type Memory struct {
	collections map[string][]Document
	dimensions  map[string]int
}

func NewMemory() *Memory {
	return &Memory{
		collections: make(map[string][]Document),
		dimensions:  make(map[string]int),
	}
}

func (m *Memory) Open(config Config) error   { return nil }
func (m *Memory) Close() error               { return nil }

func (m *Memory) CreateCollection(name string, dim int, metric string) error {
	m.collections[name] = nil
	m.dimensions[name] = dim
	return nil
}

func (m *Memory) DeleteCollection(name string) error {
	delete(m.collections, name)
	delete(m.dimensions, name)
	return nil
}

func (m *Memory) ListCollections() ([]string, error) {
	var names []string
	for name := range m.collections {
		names = append(names, name)
	}
	return names, nil
}

func (m *Memory) Upsert(collection string, docs []Document) error {
	existing := m.collections[collection]
	for _, doc := range docs {
		found := false
		for i, e := range existing {
			if e.ID == doc.ID {
				existing[i] = doc
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, doc)
		}
	}
	m.collections[collection] = existing
	return nil
}

func (m *Memory) Delete(collection string, ids []string) error {
	idSet := make(map[string]bool)
	for _, id := range ids {
		idSet[id] = true
	}
	existing := m.collections[collection]
	var filtered []Document
	for _, doc := range existing {
		if !idSet[doc.ID] {
			filtered = append(filtered, doc)
		}
	}
	m.collections[collection] = filtered
	return nil
}

func (m *Memory) Search(collection string, query []float64, opts SearchOpts) ([]Result, error) {
	docs := m.collections[collection]
	var results []Result
	for _, doc := range docs {
		score := cosineSimilarity(query, doc.Vector)
		if opts.MinScore > 0 && score < opts.MinScore {
			continue
		}
		results = append(results, Result{
			ID:       doc.ID,
			Content:  doc.Content,
			Score:    score,
			Metadata: doc.Metadata,
		})
	}
	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if opts.TopK > 0 && len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results, nil
}

func (m *Memory) SearchByText(collection string, query string, opts SearchOpts) ([]Result, error) {
	// In-memory fallback: keyword match
	docs := m.collections[collection]
	var results []Result
	queryLower := toLower(query)
	for _, doc := range docs {
		if contains(toLower(doc.Content), queryLower) {
			results = append(results, Result{
				ID:       doc.ID,
				Content:  doc.Content,
				Score:    1.0,
				Metadata: doc.Metadata,
			})
		}
	}
	if opts.TopK > 0 && len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
