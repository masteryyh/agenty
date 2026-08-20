package utils

import (
	"fmt"
	"strings"
	"unicode"
)

type ApplyDiffMode uint8

const (
	ApplyDiffDefault ApplyDiffMode = iota
	ApplyDiffCreate
)

type applyDiffChunk struct {
	originalIndex int
	deletedLines  []string
	insertedLines []string
}

type applyDiffParser struct {
	lines []string
	index int
	fuzz  int
}

const (
	applyDiffEndPatch = "*** End Patch"
	applyDiffEndFile  = "*** End of File"
)

var applyDiffSectionMarkers = []string{
	applyDiffEndPatch,
	"*** Update File:",
	"*** Delete File:",
	"*** Add File:",
	applyDiffEndFile,
}

var applyDiffSectionTerminators = []string{
	applyDiffEndPatch,
	"*** Update File:",
	"*** Delete File:",
	"*** Add File:",
}

// ApplyDiff applies a headerless V4A diff using the OpenAI Agents SDK semantics.
func ApplyDiff(input, diff string, mode ApplyDiffMode) (string, error) {
	diffLines := normalizeApplyDiffLines(diff)
	switch mode {
	case ApplyDiffCreate:
		return parseCreateDiff(diffLines)
	case ApplyDiffDefault:
	default:
		return "", fmt.Errorf("apply diff: unsupported mode %d", mode)
	}

	chunks, err := parseUpdateDiff(diffLines, input)
	if err != nil {
		return "", err
	}
	return applyDiffChunks(input, chunks)
}

func normalizeApplyDiffLines(diff string) []string {
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func parseCreateDiff(lines []string) (string, error) {
	parser := applyDiffParser{
		lines: append(append([]string{}, lines...), applyDiffEndPatch),
	}
	output := make([]string, 0, len(lines))
	for !parser.done(applyDiffSectionTerminators) {
		line := parser.lines[parser.index]
		parser.index++
		if !strings.HasPrefix(line, "+") {
			return "", fmt.Errorf("invalid add file line: %s", line)
		}
		output = append(output, strings.TrimPrefix(line, "+"))
	}
	return strings.Join(output, "\n"), nil
}

func parseUpdateDiff(lines []string, input string) ([]applyDiffChunk, error) {
	parser := applyDiffParser{
		lines: append(append([]string{}, lines...), applyDiffEndPatch),
	}
	inputLines := strings.Split(input, "\n")
	chunks := make([]applyDiffChunk, 0)
	cursor := 0

	for !parser.done(applyDiffSectionMarkers) {
		anchors, anchorCount := parser.readAnchors()
		if anchorCount == 0 && cursor != 0 {
			return nil, fmt.Errorf("invalid line:\n%s", parser.lines[parser.index])
		}

		requireAnchorMatch := anchorCount > 1
		for index, anchor := range anchors {
			var err error
			cursor, err = parser.advanceCursorToAnchor(
				anchor,
				inputLines,
				cursor,
				requireAnchorMatch,
				index > 0,
			)
			if err != nil {
				return nil, err
			}
		}

		context, sectionChunks, endIndex, eof, err := readApplyDiffSection(parser.lines, parser.index)
		if err != nil {
			return nil, err
		}
		newIndex, fuzz := findApplyDiffContext(inputLines, context, cursor, eof)
		if newIndex == -1 {
			contextText := strings.Join(context, "\n")
			if eof {
				return nil, fmt.Errorf("invalid EOF context %d:\n%s", cursor, contextText)
			}
			return nil, fmt.Errorf("invalid context %d:\n%s", cursor, contextText)
		}

		parser.fuzz += fuzz
		for _, chunk := range sectionChunks {
			chunk.originalIndex += newIndex
			chunks = append(chunks, chunk)
		}
		cursor = newIndex + len(context)
		parser.index = endIndex
	}

	return chunks, nil
}

func (parser *applyDiffParser) done(prefixes []string) bool {
	if parser.index >= len(parser.lines) {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(parser.lines[parser.index], prefix) {
			return true
		}
	}
	return false
}

func (parser *applyDiffParser) readAnchors() ([]string, int) {
	anchors := make([]string, 0)
	anchorCount := 0
	for {
		line := parser.lines[parser.index]
		switch {
		case strings.HasPrefix(line, "@@ "):
			parser.index++
			anchorCount++
			anchor := strings.TrimPrefix(line, "@@ ")
			if strings.TrimSpace(anchor) != "" {
				anchors = append(anchors, anchor)
			}
		case line == "@@":
			parser.index++
			anchorCount++
		default:
			return anchors, anchorCount
		}
	}
}

func (parser *applyDiffParser) advanceCursorToAnchor(
	anchor string,
	inputLines []string,
	cursor int,
	requireMatch bool,
	forceForwardSearch bool,
) (int, error) {
	found := false
	if !forceForwardSearch && containsApplyDiffLine(inputLines[:cursor], anchor, false) {
		found = true
	} else if index := findApplyDiffLine(inputLines, anchor, cursor, false); index >= 0 {
		cursor = index + 1
		found = true
	}

	if !found {
		if !forceForwardSearch && containsApplyDiffLine(inputLines[:cursor], anchor, true) {
			found = true
		} else if index := findApplyDiffLine(inputLines, anchor, cursor, true); index >= 0 {
			cursor = index + 1
			parser.fuzz++
			found = true
		}
	}

	if requireMatch && !found {
		return 0, fmt.Errorf("invalid anchor %d:\n%s", cursor, anchor)
	}
	return cursor, nil
}

func containsApplyDiffLine(lines []string, target string, trimmed bool) bool {
	return findApplyDiffLine(lines, target, 0, trimmed) >= 0
}

func findApplyDiffLine(lines []string, target string, start int, trimmed bool) int {
	for index := start; index < len(lines); index++ {
		line := lines[index]
		if trimmed {
			line = strings.TrimSpace(line)
			target = strings.TrimSpace(target)
		}
		if line == target {
			return index
		}
	}
	return -1
}

func readApplyDiffSection(
	lines []string,
	startIndex int,
) ([]string, []applyDiffChunk, int, bool, error) {
	context := make([]string, 0)
	deletedLines := make([]string, 0)
	insertedLines := make([]string, 0)
	chunks := make([]applyDiffChunk, 0)
	mode := byte(' ')
	index := startIndex

	flushChunk := func() {
		if len(insertedLines) == 0 && len(deletedLines) == 0 {
			return
		}
		chunks = append(chunks, applyDiffChunk{
			originalIndex: len(context) - len(deletedLines),
			deletedLines:  deletedLines,
			insertedLines: insertedLines,
		})
		deletedLines = make([]string, 0)
		insertedLines = make([]string, 0)
	}

	for index < len(lines) {
		raw := lines[index]
		if strings.HasPrefix(raw, "@@") || isApplyDiffSectionEnd(raw) {
			break
		}
		if raw == "***" {
			break
		}
		if strings.HasPrefix(raw, "***") {
			return nil, nil, 0, false, fmt.Errorf("invalid line: %s", raw)
		}

		index++
		previousMode := mode
		line := raw
		if line == "" {
			line = " "
		}
		switch line[0] {
		case '+', '-', ' ':
			mode = line[0]
		default:
			return nil, nil, 0, false, fmt.Errorf("invalid line: %s", line)
		}
		line = line[1:]

		if mode == ' ' && previousMode != mode {
			flushChunk()
		}
		switch mode {
		case '-':
			deletedLines = append(deletedLines, line)
			context = append(context, line)
		case '+':
			insertedLines = append(insertedLines, line)
		case ' ':
			context = append(context, line)
		}
	}
	flushChunk()

	if index < len(lines) && lines[index] == applyDiffEndFile {
		return context, chunks, index + 1, true, nil
	}
	if index == startIndex {
		return nil, nil, 0, false, fmt.Errorf("nothing in section at index %d: %s", index, lines[index])
	}
	return context, chunks, index, false, nil
}

func isApplyDiffSectionEnd(line string) bool {
	for _, marker := range applyDiffSectionMarkers {
		if strings.HasPrefix(line, marker) {
			return true
		}
	}
	return false
}

func findApplyDiffContext(lines, context []string, start int, eof bool) (int, int) {
	if eof {
		endStart := max(0, len(lines)-len(context))
		if index, fuzz := findApplyDiffContextCore(lines, context, endStart); index != -1 {
			return index, fuzz
		}
		index, fuzz := findApplyDiffContextCore(lines, context, start)
		return index, fuzz + 10_000
	}
	return findApplyDiffContextCore(lines, context, start)
}

func findApplyDiffContextCore(lines, context []string, start int) (int, int) {
	if len(context) == 0 {
		return start, 0
	}

	comparisons := []struct {
		fuzz int
		mapf func(string) string
	}{
		{fuzz: 0, mapf: func(value string) string { return value }},
		{fuzz: 1, mapf: func(value string) string {
			return strings.TrimRightFunc(value, unicode.IsSpace)
		}},
		{fuzz: 100, mapf: strings.TrimSpace},
	}
	for _, comparison := range comparisons {
		for index := start; index < len(lines); index++ {
			if equalApplyDiffSlice(lines, context, index, comparison.mapf) {
				return index, comparison.fuzz
			}
		}
	}
	return -1, 0
}

func equalApplyDiffSlice(
	source []string,
	target []string,
	start int,
	mapf func(string) string,
) bool {
	if start+len(target) > len(source) {
		return false
	}
	for index := range target {
		if mapf(source[start+index]) != mapf(target[index]) {
			return false
		}
	}
	return true
}

func applyDiffChunks(input string, chunks []applyDiffChunk) (string, error) {
	originalLines := strings.Split(input, "\n")
	destinationLines := make([]string, 0, len(originalLines))
	originalIndex := 0
	for _, chunk := range chunks {
		if chunk.originalIndex > len(originalLines) {
			return "", fmt.Errorf(
				"applyDiff: chunk original index %d exceeds input length %d",
				chunk.originalIndex,
				len(originalLines),
			)
		}
		if originalIndex > chunk.originalIndex {
			return "", fmt.Errorf(
				"applyDiff: overlapping chunk at %d with cursor %d",
				chunk.originalIndex,
				originalIndex,
			)
		}

		destinationLines = append(destinationLines, originalLines[originalIndex:chunk.originalIndex]...)
		destinationLines = append(destinationLines, chunk.insertedLines...)
		originalIndex = chunk.originalIndex + len(chunk.deletedLines)
	}
	destinationLines = append(destinationLines, originalLines[originalIndex:]...)
	return strings.Join(destinationLines, "\n"), nil
}
