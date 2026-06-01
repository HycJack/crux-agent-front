package primitives

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/hermes-go/core/types"
)

// FilePrimitives provides built-in file operations.
type FilePrimitives struct {
	// AllowedPaths restricts file operations to these prefixes. Empty = no restriction.
	AllowedPaths []string
}

func NewFile() *FilePrimitives {
	return &FilePrimitives{}
}

func NewFileSandbox(allowedPaths []string) *FilePrimitives {
	return &FilePrimitives{AllowedPaths: allowedPaths}
}

func (fp *FilePrimitives) ToolSchemas() []types.ToolSchema {
	return []types.ToolSchema{
		{
			Name:        "read_file",
			Description: "Read a file with line numbers. Use offset/limit for large files.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "File path"},
					"offset": map[string]any{"type": "integer", "description": "Start line (1-indexed)", "default": 1},
					"limit":  map[string]any{"type": "integer", "description": "Max lines", "default": 500},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file, creating directories as needed.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "File path"},
					"content": map[string]any{"type": "string", "description": "Content to write"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "patch_file",
			Description: "Find and replace text in a file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":       map[string]any{"type": "string", "description": "File path"},
					"old_string": map[string]any{"type": "string", "description": "Text to find"},
					"new_string": map[string]any{"type": "string", "description": "Replacement text"},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
		},
		{
			Name:        "search_content",
			Description: "Search for text patterns inside files using regex.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Regex pattern"},
					"path":    map[string]any{"type": "string", "description": "Directory to search", "default": "."},
					"limit":   map[string]any{"type": "integer", "description": "Max results", "default": 20},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "search_files",
			Description: "Find files by name pattern.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Glob pattern"},
					"path":    map[string]any{"type": "string", "description": "Directory", "default": "."},
					"limit":   map[string]any{"type": "integer", "description": "Max results", "default": 20},
				},
				"required": []string{"pattern"},
			},
		},
	}
}

func (fp *FilePrimitives) Handle(call types.ToolCall) types.ToolResult {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return types.ToolResult{ToolCallID: call.ID, Content: fmt.Sprintf("Error: invalid arguments: %v", err), IsError: true}
	}

	var content string
	var err error

	switch call.Function.Name {
	case "read_file":
		content, err = fp.readFile(args)
	case "write_file":
		content, err = fp.writeFile(args)
	case "patch_file":
		content, err = fp.patchFile(args)
	case "search_content":
		content, err = fp.searchContent(args)
	case "search_files":
		content, err = fp.searchFiles(args)
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

// validatePath checks if a path is within allowed directories.
func (fp *FilePrimitives) validatePath(path string) error {
	if len(fp.AllowedPaths) == 0 {
		return nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	for _, allowed := range fp.AllowedPaths {
		absAllowed, _ := filepath.Abs(allowed)
		if strings.HasPrefix(absPath, absAllowed) {
			return nil
		}
	}
	return fmt.Errorf("path %q is outside allowed directories", path)
}

func (fp *FilePrimitives) readFile(args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := fp.validatePath(path); err != nil {
		return "", err
	}

	offset := intArg(args, "offset", 1)
	limit := intArg(args, "limit", 500)

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if len(lines) >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("%d|%s", lineNum, scanner.Text()))
	}

	if len(lines) == 0 {
		return fmt.Sprintf("(file has %d lines, offset %d is beyond end)", lineNum, offset), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Total lines: %d\n", lineNum))
	sb.WriteString(strings.Join(lines, "\n"))
	return sb.String(), nil
}

func (fp *FilePrimitives) writeFile(args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := fp.validatePath(path); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	runeCount := utf8.RuneCountInString(content)
	return fmt.Sprintf("Wrote %d bytes (%d runes) to %s", len(content), runeCount, path), nil
}

func (fp *FilePrimitives) patchFile(args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)
	if path == "" || oldStr == "" {
		return "", fmt.Errorf("path and old_string are required")
	}
	if err := fp.validatePath(path); err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := string(data)
	if !strings.Contains(content, oldStr) {
		return "", fmt.Errorf("old_string not found in %s", path)
	}

	result := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Patched %s", path), nil
}

func (fp *FilePrimitives) searchContent(args map[string]any) (string, error) {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)
	limit := intArg(args, "limit", 20)

	if path == "" {
		path = "."
	}
	if err := fp.validatePath(path); err != nil {
		return "", err
	}

	cmd := exec.Command("grep", "-rn", "-m", fmt.Sprintf("%d", limit), "--include=*", pattern, path)
	out, _ := cmd.CombinedOutput()
	if len(out) == 0 {
		return "No matches found.", nil
	}
	// Truncate if too long
	result := string(out)
	if len(result) > 8000 {
		result = result[:8000] + "\n...(truncated)"
	}
	return result, nil
}

func (fp *FilePrimitives) searchFiles(args map[string]any) (string, error) {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)
	limit := intArg(args, "limit", 20)

	if path == "" {
		path = "."
	}
	if err := fp.validatePath(path); err != nil {
		return "", err
	}

	matches, err := filepath.Glob(filepath.Join(path, pattern))
	if err != nil {
		return "", err
	}

	if len(matches) > limit {
		matches = matches[:limit]
	}
	if len(matches) == 0 {
		return "No files found.", nil
	}
	return strings.Join(matches, "\n"), nil
}

func intArg(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	default:
		return defaultVal
	}
}
