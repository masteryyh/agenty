/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package chunk

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	tableLineRe   = regexp.MustCompile(`(?m)^\s*\|`)
	codeFenceRe   = regexp.MustCompile("(?m)^```")
	sentenceEndRe = regexp.MustCompile(`[.!?。！？]["']?\s+`)
)

// SplitText splits text into overlapping chunks while preserving semantic units
// such as markdown tables and code fences.
func SplitText(text string, chunkSize, overlap int) []string {
	if len(text) < chunkSize {
		return []string{text}
	}

	if chunkSize <= 0 {
		chunkSize = 512
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 0
	}

	segments := splitIntoSegments(text)
	if len(segments) == 0 {
		return nil
	}

	var chunks []string
	var currentWords []string

	for _, seg := range segments {
		words := tokenize(seg)
		if len(words) == 0 {
			continue
		}

		if len(words) > chunkSize {
			if len(currentWords) > 0 {
				chunks = append(chunks, strings.Join(currentWords, " "))
				currentWords = overlapSlice(currentWords, overlap)
			}
			subChunks := splitLargeSegment(seg, chunkSize, overlap)
			chunks = append(chunks, subChunks...)
			continue
		}

		if len(currentWords)+len(words) > chunkSize && len(currentWords) > 0 {
			chunks = append(chunks, strings.Join(currentWords, " "))
			currentWords = overlapSlice(currentWords, overlap)
		}

		currentWords = append(currentWords, words...)
	}

	if len(currentWords) > 0 {
		chunks = append(chunks, strings.Join(currentWords, " "))
	}

	return chunks
}

func splitIntoSegments(text string) []string {
	var segments []string
	var currentLines []string
	inCodeFence := false

	lines := strings.Split(text, "\n")

	flushLines := func() {
		if len(currentLines) == 0 {
			return
		}
		block := strings.Join(currentLines, "\n")
		block = strings.TrimSpace(block)
		if block != "" {
			segments = append(segments, block)
		}
		currentLines = nil
	}

	i := 0
	for i < len(lines) {
		line := lines[i]

		if codeFenceRe.MatchString(line) {
			if inCodeFence {
				currentLines = append(currentLines, line)
				inCodeFence = false
				flushLines()
			} else {
				flushLines()
				inCodeFence = true
				currentLines = append(currentLines, line)
			}
			i++
			continue
		}

		if inCodeFence {
			currentLines = append(currentLines, line)
			i++
			continue
		}

		if tableLineRe.MatchString(line) {
			flushLines()
			var tableLines []string
			for i < len(lines) && tableLineRe.MatchString(lines[i]) {
				tableLines = append(tableLines, lines[i])
				i++
			}
			segments = append(segments, strings.Join(tableLines, "\n"))
			continue
		}

		if strings.TrimSpace(line) == "" {
			flushLines()
			i++
			continue
		}

		currentLines = append(currentLines, line)
		i++
	}

	flushLines()
	return segments
}

func splitLargeSegment(seg string, chunkSize, overlap int) []string {
	sentences := sentenceEndRe.Split(seg, -1)

	var chunks []string
	var currentWords []string

	for _, sent := range sentences {
		words := tokenize(sent)
		if len(words) == 0 {
			continue
		}

		if len(currentWords)+len(words) > chunkSize && len(currentWords) > 0 {
			chunks = append(chunks, strings.Join(currentWords, " "))
			currentWords = overlapSlice(currentWords, overlap)
		}

		if len(words) > chunkSize {
			step := chunkSize - overlap
			if step <= 0 {
				step = 1
			}
			for j := 0; j < len(words); j += step {
				end := min(j + chunkSize, len(words))
				chunks = append(chunks, strings.Join(words[j:end], " "))
				if end == len(words) {
					break
				}
			}
			continue
		}

		currentWords = append(currentWords, words...)
	}

	if len(currentWords) > 0 {
		chunks = append(chunks, strings.Join(currentWords, " "))
	}

	return chunks
}

func overlapSlice(words []string, overlap int) []string {
	if overlap <= 0 || len(words) <= overlap {
		return nil
	}
	result := make([]string, overlap)
	copy(result, words[len(words)-overlap:])
	return result
}

func tokenize(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}
