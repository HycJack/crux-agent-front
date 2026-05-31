package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ════════════════════════════════════════════════
// Web Search Skill — web_search + fetch_url
// ════════════════════════════════════════════════

type WebSearchSkill struct {
	client *http.Client
}

func NewWebSearchSkill() *WebSearchSkill {
	return &WebSearchSkill{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (w *WebSearchSkill) ToolSchemas() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "web_search",
				"description": "Search the web for up-to-date information. Returns titles, URLs, and snippets. Use when you need current events, real-time data, or when your knowledge might be outdated.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Max results to return (default 5, max 10)",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "fetch_url",
				"description": "Fetch and extract text content from a URL. Useful for reading articles, documentation, or any web page. Returns cleaned text content.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "URL to fetch",
						},
						"max_chars": map[string]interface{}{
							"type":        "integer",
							"description": "Max characters to return (default 5000)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
	}
}

func (w *WebSearchSkill) ToolNames() []string {
	return []string{"web_search", "fetch_url"}
}

func (w *WebSearchSkill) Handle(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "web_search":
		return w.search(args)
	case "fetch_url":
		return w.fetch(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

func (w *WebSearchSkill) search(args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	limit := 5
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
		if limit > 10 {
			limit = 10
		}
	}

	// Use DuckDuckGo HTML (no API key needed)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CruxChat/1.0)")

	resp, err := w.client.Do(req)
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
		title := stripHTMLTags(titles[i][2])
		href := titles[i][1]
		// Decode DuckDuckGo redirect URL
		if strings.Contains(href, "uddg=") {
			if u, err := url.Parse(href); err == nil {
				if realURL := u.Query().Get("uddg"); realURL != "" {
					href = realURL
				}
			}
		}
		snippet := ""
		if i < len(snippets) {
			snippet = stripHTMLTags(snippets[i][1])
		}
		results = append(results, fmt.Sprintf("%d. %s\n   URL: %s\n   %s", i+1, title, href, snippet))
	}

	if len(results) == 0 {
		return "No results found.", nil
	}
	return strings.Join(results, "\n\n"), nil
}

func (w *WebSearchSkill) fetch(args map[string]interface{}) (string, error) {
	u, _ := args["url"].(string)
	if u == "" {
		return "", fmt.Errorf("url is required")
	}
	maxChars := 5000
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
		if maxChars > 20000 {
			maxChars = 20000
		}
	}

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CruxChat/1.0)")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 200000)) // 200KB limit
	if err != nil {
		return "", err
	}

	text := stripHTMLTags(string(body))
	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")

	if len(text) > maxChars {
		text = text[:maxChars] + "\n...(truncated)"
	}
	return text, nil
}

func stripHTMLTags(s string) string {
	tagRe := regexp.MustCompile(`<[^>]*>`)
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
