package docparse

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type markdownFrontmatter struct {
	Title  string `yaml:"title"`
	Author string `yaml:"author"`
}

// splitFrontmatter strips a leading "---"-delimited YAML block, if any, and
// returns the parsed frontmatter alongside the remaining body.
func splitFrontmatter(text string) (markdownFrontmatter, string) {
	var fm markdownFrontmatter
	if !strings.HasPrefix(text, "---\n") && text != "---" {
		return fm, text
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fm, text
	}
	block := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	_ = yaml.Unmarshal([]byte(block), &fm)
	return fm, body
}

// markdownHeadingLevel returns the ATX heading level (1-6) of a line, or 0
// if it is not a heading.
func markdownHeadingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0
	}
	if level == len(trimmed) {
		return level // "#" alone
	}
	if trimmed[level] != ' ' && trimmed[level] != '\t' {
		return 0
	}
	return level
}

// splitMarkdownChapters splits on ATX headings of the given level, skipping
// any "#" found inside fenced code blocks (``` or ~~~).
func splitMarkdownChapters(body string, level int) []Chapter {
	lines := strings.Split(body, "\n")
	var starts []int
	inFence := false
	var fenceMarker string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			if strings.HasPrefix(trimmed, "```") {
				fenceMarker = "```"
			} else {
				fenceMarker = "~~~"
			}
			continue
		}
		if markdownHeadingLevel(line) == level {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return nil
	}
	chapters := make([]Chapter, 0, len(starts))
	for i, start := range starts {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		segment := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		title := strings.TrimSpace(strings.TrimLeft(lines[start], " #"))
		chapters = append(chapters, Chapter{Index: i + 1, Title: title, Text: segment})
	}
	return chapters
}

func extractMarkdown(path string) (Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := normalizeNewlines(string(b))
	fm, body := splitFrontmatter(text)

	title := fm.Title
	if title == "" {
		title = stripExt(filepath.Base(path))
	}

	chapters := splitMarkdownChapters(body, 1)
	if chapters == nil {
		chapters = splitMarkdownChapters(body, 2)
	}
	if chapters == nil {
		chapters = []Chapter{{Index: 1, Text: strings.TrimSpace(body)}}
	}

	return Document{
		Title:    title,
		Author:   fm.Author,
		Format:   "markdown",
		Chapters: chapters,
	}, nil
}
