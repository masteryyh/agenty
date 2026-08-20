package builtin

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/masteryyh/agenty-core/pkg/agentloop"
	"github.com/masteryyh/agenty-core/pkg/domain/conversation"
)

var errResultLimitReached = errors.New("builtin: result limit reached")

type grepTool struct {
	fileSystem *fileSystem
}

type grepArguments struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path,omitempty"`
	Glob          string `json:"glob,omitempty"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"`
	MaxResults    *int   `json:"max_results,omitempty"`
}

type grepMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

type grepResult struct {
	Root      string      `json:"root"`
	Matches   []grepMatch `json:"matches"`
	Truncated bool        `json:"truncated"`
}

func (tool *grepTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name: "grep",
		Description: "Recursively search text files with a Go regular expression. " +
			"An optional glob filters relative file paths, and results include path, line, column, and text.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"pattern":        stringSchema("Go regular expression to search for."),
				"path":           stringSchema("File or directory to search. Defaults to the session working directory."),
				"glob":           stringSchema("Optional relative path glob. Use ** as a complete segment for recursion."),
				"case_sensitive": booleanSchema("Whether matching is case-sensitive. Defaults to true."),
				"max_results":    integerSchema("Maximum matching lines to return. Defaults to 200 and cannot exceed 1000.", 1),
			},
			[]string{"pattern"},
		),
	}
}

func (tool *grepTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments grepArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("grep: %w", err)
	}
	if arguments.Pattern == "" {
		return nil, fmt.Errorf("grep: pattern must not be empty")
	}

	limit, err := normalizeMaxResults(arguments.MaxResults)
	if err != nil {
		return nil, fmt.Errorf("grep: %w", err)
	}
	root, err := resolvePath(arguments.Path, callContext.Cwd, true)
	if err != nil {
		return nil, fmt.Errorf("grep: %w", err)
	}
	if arguments.Glob != "" {
		if err := validateGlob(arguments.Glob); err != nil {
			return nil, fmt.Errorf("grep: invalid glob: %w", err)
		}
	}

	pattern := arguments.Pattern
	caseSensitive := arguments.CaseSensitive == nil || *arguments.CaseSensitive
	if !caseSensitive {
		pattern = "(?i)" + pattern
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("grep: compile pattern: %w", err)
	}

	tool.fileSystem.mu.RLock()
	defer tool.fileSystem.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err := grepFiles(ctx, root, arguments.Glob, expression, limit)
	if err != nil {
		return nil, fmt.Errorf("grep: %w", err)
	}
	return resultContent(result)
}

func grepFiles(
	ctx context.Context,
	root string,
	glob string,
	expression *regexp.Regexp,
	limit int,
) (grepResult, error) {
	result := grepResult{
		Root:    root,
		Matches: []grepMatch{},
	}

	info, err := os.Stat(root)
	if err != nil {
		return grepResult{}, fmt.Errorf("inspect %q: %w", root, err)
	}
	if info.Mode().IsRegular() {
		if glob != "" {
			matches, err := matchGlob(glob, filepath.Base(root))
			if err != nil {
				return grepResult{}, err
			}
			if !matches {
				return result, nil
			}
		}
		if err := grepFile(ctx, root, expression, limit, &result); errors.Is(err, errResultLimitReached) {
			result.Matches = result.Matches[:limit]
			result.Truncated = true
		} else if err != nil {
			return grepResult{}, err
		}
		return result, nil
	}
	if !info.IsDir() {
		return grepResult{}, fmt.Errorf("path %q is not a regular file or directory", root)
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if glob != "" {
			matches, err := matchGlob(glob, filepath.ToSlash(relative))
			if err != nil {
				return err
			}
			if !matches {
				return nil
			}
		}

		return grepFile(ctx, path, expression, limit, &result)
	})
	if errors.Is(err, errResultLimitReached) {
		result.Matches = result.Matches[:limit]
		result.Truncated = true
		return result, nil
	}
	if err != nil {
		return grepResult{}, fmt.Errorf("walk %q: %w", root, err)
	}
	return result, nil
}

func grepFile(
	ctx context.Context,
	path string,
	expression *regexp.Regexp,
	limit int,
	result *grepResult,
) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 8*1024)
	prefix, peekErr := reader.Peek(8 * 1024)
	if peekErr != nil && !errors.Is(peekErr, io.EOF) && !errors.Is(peekErr, bufio.ErrBufferFull) {
		return fmt.Errorf("inspect %q: %w", path, peekErr)
	}
	if bytes.IndexByte(prefix, 0) >= 0 {
		return nil
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxScannerTokenSize)
	lineNumber := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		lineNumber++
		location := expression.FindStringIndex(scanner.Text())
		if location == nil {
			continue
		}
		result.Matches = append(result.Matches, grepMatch{
			Path:   path,
			Line:   lineNumber,
			Column: utf8Column(scanner.Text(), location[0]),
			Text:   scanner.Text(),
		})
		if len(result.Matches) > limit {
			return errResultLimitReached
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %q: %w", path, err)
	}
	return nil
}

func utf8Column(line string, byteOffset int) int {
	return len([]rune(line[:byteOffset])) + 1
}

type globTool struct {
	fileSystem *fileSystem
}

type globArguments struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	MaxResults *int   `json:"max_results,omitempty"`
}

type globResult struct {
	Root      string   `json:"root"`
	Paths     []string `json:"paths"`
	Truncated bool     `json:"truncated"`
}

func (tool *globTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name: "glob",
		Description: "Find files and symbolic links whose relative paths match a glob. " +
			"Use ** as a complete path segment for recursive matching. Results are sorted.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"pattern":     stringSchema("Relative path glob, for example **/*.go or pkg/*/README.md."),
				"path":        stringSchema("Directory to search. Defaults to the session working directory."),
				"max_results": integerSchema("Maximum paths to return. Defaults to 200 and cannot exceed 1000.", 1),
			},
			[]string{"pattern"},
		),
	}
}

func (tool *globTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments globArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}
	if strings.TrimSpace(arguments.Pattern) == "" {
		return nil, fmt.Errorf("glob: pattern must not be empty")
	}
	if err := validateGlob(arguments.Pattern); err != nil {
		return nil, fmt.Errorf("glob: invalid pattern: %w", err)
	}

	limit, err := normalizeMaxResults(arguments.MaxResults)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}
	root, err := resolvePath(arguments.Path, callContext.Cwd, true)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}

	tool.fileSystem.mu.RLock()
	defer tool.fileSystem.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err := findGlobMatches(ctx, root, arguments.Pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}
	return resultContent(result)
}

func findGlobMatches(
	ctx context.Context,
	root string,
	pattern string,
	limit int,
) (globResult, error) {
	result := globResult{
		Root:  root,
		Paths: []string{},
	}

	info, err := os.Stat(root)
	if err != nil {
		return globResult{}, fmt.Errorf("inspect %q: %w", root, err)
	}
	if !info.IsDir() {
		return globResult{}, fmt.Errorf("path %q is not a directory", root)
	}

	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink == 0 {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		matches, err := matchGlob(pattern, filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		if !matches {
			return nil
		}

		result.Paths = append(result.Paths, path)
		if len(result.Paths) > limit {
			return errResultLimitReached
		}
		return nil
	})
	if errors.Is(err, errResultLimitReached) {
		result.Paths = result.Paths[:limit]
		result.Truncated = true
	} else if err != nil {
		return globResult{}, fmt.Errorf("walk %q: %w", root, err)
	}

	sort.Strings(result.Paths)
	return result, nil
}

func validateGlob(pattern string) error {
	if filepath.IsAbs(strings.TrimSpace(pattern)) {
		return fmt.Errorf("pattern must be relative to path")
	}

	pattern = normalizeGlob(pattern)
	if pattern == "" {
		return fmt.Errorf("pattern must not be empty")
	}
	for segment := range strings.SplitSeq(pattern, "/") {
		if segment == "**" {
			continue
		}
		if _, err := pathpkg.Match(segment, ""); err != nil {
			return err
		}
	}
	return nil
}

func matchGlob(pattern, relativePath string) (bool, error) {
	patternSegments := strings.Split(normalizeGlob(pattern), "/")
	pathSegments := strings.Split(filepath.ToSlash(relativePath), "/")
	type state struct {
		pattern int
		path    int
	}
	memo := make(map[state]bool)
	seen := make(map[state]bool)

	var match func(int, int) (bool, error)
	match = func(patternIndex, pathIndex int) (bool, error) {
		current := state{pattern: patternIndex, path: pathIndex}
		if seen[current] {
			return memo[current], nil
		}
		seen[current] = true

		if patternIndex == len(patternSegments) {
			memo[current] = pathIndex == len(pathSegments)
			return memo[current], nil
		}
		if patternSegments[patternIndex] == "**" {
			matched, err := match(patternIndex+1, pathIndex)
			if err != nil || matched {
				memo[current] = matched
				return matched, err
			}
			if pathIndex < len(pathSegments) {
				matched, err = match(patternIndex, pathIndex+1)
				memo[current] = matched
				return matched, err
			}
			return false, nil
		}
		if pathIndex == len(pathSegments) {
			return false, nil
		}

		segmentMatches, err := pathpkg.Match(patternSegments[patternIndex], pathSegments[pathIndex])
		if err != nil || !segmentMatches {
			return false, err
		}
		matched, err := match(patternIndex+1, pathIndex+1)
		memo[current] = matched
		return matched, err
	}

	return match(0, 0)
}

func normalizeGlob(pattern string) string {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	return strings.TrimPrefix(pattern, "./")
}

type listTool struct {
	fileSystem *fileSystem
}

type listArguments struct {
	Path string `json:"path,omitempty"`
}

type listEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type listResult struct {
	Path      string      `json:"path"`
	Entries   []listEntry `json:"entries"`
	Truncated bool        `json:"truncated"`
}

func (tool *listTool) Definition() agentloop.ToolDefinition {
	return agentloop.ToolDefinition{
		Name:        "ls",
		Description: "List the immediate children of a directory in name order, including entry type and size.",
		InputSchema: objectSchema(
			map[string]agentloop.JSONSchema{
				"path": stringSchema("Directory to list. Defaults to the session working directory."),
			},
			nil,
		),
	}
}

func (tool *listTool) Execute(
	ctx context.Context,
	callContext agentloop.CallContext,
	input []byte,
) (conversation.Content, error) {
	var arguments listArguments
	if err := decodeArguments(input, &arguments); err != nil {
		return nil, fmt.Errorf("ls: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := resolvePath(arguments.Path, callContext.Cwd, true)
	if err != nil {
		return nil, fmt.Errorf("ls: %w", err)
	}

	tool.fileSystem.mu.RLock()
	defer tool.fileSystem.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err := listDirectory(path)
	if err != nil {
		return nil, fmt.Errorf("ls: %w", err)
	}
	return resultContent(result)
}

func listDirectory(path string) (listResult, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return listResult{}, fmt.Errorf("read directory %q: %w", path, err)
	}

	result := listResult{
		Path:    path,
		Entries: make([]listEntry, 0, min(len(entries), maxResults)),
	}
	for _, entry := range entries {
		if len(result.Entries) == maxResults {
			result.Truncated = true
			break
		}

		info, err := entry.Info()
		if err != nil {
			return listResult{}, fmt.Errorf("inspect %q: %w", filepath.Join(path, entry.Name()), err)
		}
		entryType := "file"
		switch {
		case info.IsDir():
			entryType = "directory"
		case info.Mode()&os.ModeSymlink != 0:
			entryType = "symlink"
		case !info.Mode().IsRegular():
			entryType = "other"
		}
		result.Entries = append(result.Entries, listEntry{
			Name: entry.Name(),
			Type: entryType,
			Size: info.Size(),
		})
	}
	return result, nil
}
