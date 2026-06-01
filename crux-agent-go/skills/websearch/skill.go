package websearch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hermes-go/core/types"
)

// Skill provides web search capability.
type Skill struct {
	apiKey  string
	engine  string // "duckduckgo", "serpapi"
	client  *http.Client
}

func New(apiKey string) *Skill {
	return &Skill{
		apiKey: apiKey,
		engine: "duckduckgo",
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Skill) Name() string { return "web-search" }

func (s *Skill) Capabilities() struct{ Tools []types.ToolSchema } {
	return struct{ Tools []types.ToolSchema }{
		Tools: []types.ToolSchema{
			{
				Name:        "web_search",
				Description: "Search the web for information. Returns titles, URLs, and snippets.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string", "description": "Search query"},
						"limit": map[string]any{"type": "integer", "description": "Max results (default 5)"},
					},
					"required": []string{"query"},
				},
			},
			{
				Name:        "fetch_url",
				Description: "Fetch and extract text content from a URL.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{"type": "string", "description": "URL to fetch"},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

func (s *Skill) Handle(call types.ToolCall) types.ToolResult {
	var args map[string]any
	json.Unmarshal([]byte(call.Function.Arguments), &args)

	var content string
	var err error

	switch call.Function.Name {
	case "web_search":
		query, _ := args["query"].(string)
		limit := 5
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		content, err = s.search(query, limit)

	case "fetch_url":
		u, _ := args["url"].(string)
		content, err = s.fetch(u)

	default:
		err = fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	result := types.ToolResult{ToolCallID: call.ID}
	if err != nil {
		result.Content = fmt.Sprintf("Error: %v", err)
		result.IsError = true
	} else {
		result.Content = content
	}
	return result
}

func (s *Skill) search(query string, limit int) (string, error) {
	// Use DuckDuckGo HTML (no API key needed)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HermesBot/1.0)")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// Parse results from DuckDuckGo HTML
	titleRe := regexp.MustCompile(`<a rel="nofollow" class="result__a" href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`<a class="result__snippet"[^>]*>(.*?)</a>`)

	titles := titleRe.FindAllStringSubmatch(html, -1)
	snippets := snippetRe.FindAllStringSubmatch(html, -1)

	var results []string
	for i := 0; i < len(titles) && i < limit; i++ {
		title := stripHTML(titles[i][2])
		href := titles[i][1]
		snippet := ""
		if i < len(snippets) {
			snippet = stripHTML(snippets[i][1])
		}
		results = append(results, fmt.Sprintf("%d. %s\n   URL: %s\n   %s", i+1, title, href, snippet))
	}

	if len(results) == 0 {
		return "No results found.", nil
	}
	return strings.Join(results, "\n\n"), nil
}

func (s *Skill) fetch(u string) (string, error) {
	resp, err := s.client.Get(u)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50000)) // 50KB limit
	if err != nil {
		return "", err
	}

	// Strip HTML tags for plain text
	text := stripHTML(string(body))
	if len(text) > 5000 {
		text = text[:5000] + "\n...(truncated)"
	}
	return text, nil
}

func stripHTML(s string) string {
	tagRe := regexp.MustCompile(`<[^>]*>`)
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
