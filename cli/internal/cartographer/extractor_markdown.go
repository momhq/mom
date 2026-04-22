package cartographer

import (
	"context"
	"regexp"
	"strings"
)

// markdownExtensions is the set of file extensions handled by MarkdownExtractor.
var markdownExtensions = map[string]bool{
	".md": true, ".mdx": true, ".txt": true, ".rst": true,
}

// MarkdownExtractor extracts decisions, patterns, and facts from markdown-like files.
type MarkdownExtractor struct{}

// NewMarkdownExtractor returns an initialised MarkdownExtractor.
func NewMarkdownExtractor() *MarkdownExtractor { return &MarkdownExtractor{} }

func (e *MarkdownExtractor) Name() string { return "markdown" }

func (e *MarkdownExtractor) Matches(path string) bool {
	ext := strings.ToLower(fileExt(path))
	return markdownExtensions[ext]
}

// Patterns used for extraction.
var (
	reDecisionInline = regexp.MustCompile(`(?i)^Decision:\s*(.+)`)
	rePatternInline  = regexp.MustCompile(`(?i)^Pattern:\s*(.+)`)
	reURL = regexp.MustCompile(`https?://[^\s)\]"']+`)
)

func (e *MarkdownExtractor) Extract(_ context.Context, src Source) ([]Draft, error) {
	lines := linesOf(src.Content)
	srcHash := hashBytes(src.Content)

	var drafts []Draft

	// Track current heading context to detect section-level decisions/patterns.
	type section struct {
		heading string
		kind    string // "decision" | "pattern" | ""
		start   int
		body    strings.Builder
	}

	var cur *section

	flush := func(endLine int) {
		if cur == nil {
			return
		}
		body := strings.TrimSpace(cur.body.String())
		if body == "" || cur.kind == "" {
			cur = nil
			return
		}
		d := Draft{
			Summary: truncate(body, 120),
			Tags:    []string{cur.kind, "bootstrap"},
			Content: map[string]any{
				"heading": cur.heading,
				"body":    body,
			},
			Provenance: ProvenanceMeta{
				SourceFile:   src.Path,
				SourceLines:  lineRange(cur.start+1, endLine),
				SourceHash:   srcHash,
				TriggerEvent: TriggerEvent,
			},
		}
		drafts = append(drafts, d)
		cur = nil
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect ATX headings (# ## ###).
		if strings.HasPrefix(trimmed, "#") {
			flush(i)

			heading := strings.TrimLeft(trimmed, "# ")
			var kind string
			lower := strings.ToLower(heading)
			switch {
			case strings.Contains(lower, "decision"):
				kind = "decision"
			case strings.Contains(lower, "pattern"):
				kind = "pattern"
			case strings.Contains(lower, "architecture"), strings.Contains(lower, "adr"):
				kind = "decision"
			}
			cur = &section{heading: heading, kind: kind, start: i}
			continue
		}

		// Detect inline "Decision: ..." and "Pattern: ..." markers.
		if m := reDecisionInline.FindStringSubmatch(trimmed); m != nil {
			flush(i)
			drafts = append(drafts, Draft{
				Summary: truncate(m[1], 120),
				Tags:    []string{"decision", "bootstrap"},
				Content: map[string]any{"text": m[1]},
				Provenance: ProvenanceMeta{
					SourceFile:   src.Path,
					SourceLines:  lineRange(i+1, i+1),
					SourceHash:   srcHash,
					TriggerEvent: TriggerEvent,
				},
			})
			continue
		}

		if m := rePatternInline.FindStringSubmatch(trimmed); m != nil {
			flush(i)
			drafts = append(drafts, Draft{
				Summary: truncate(m[1], 120),
				Tags:    []string{"pattern", "bootstrap"},
				Content: map[string]any{"text": m[1]},
				Provenance: ProvenanceMeta{
					SourceFile:   src.Path,
					SourceLines:  lineRange(i+1, i+1),
					SourceHash:   srcHash,
					TriggerEvent: TriggerEvent,
				},
			})
			continue
		}

		// Extract URLs as facts.
		urls := reURL.FindAllString(trimmed, -1)
		for _, u := range urls {
			drafts = append(drafts, Draft{
				Summary: "URL reference: " + truncate(u, 100),
				Tags:    []string{"fact", "url", "bootstrap"},
				Content: map[string]any{"url": u},
				Provenance: ProvenanceMeta{
					SourceFile:   src.Path,
					SourceLines:  lineRange(i+1, i+1),
					SourceHash:   srcHash,
					TriggerEvent: TriggerEvent,
				},
			})
		}

		// Accumulate body for current section.
		if cur != nil {
			cur.body.WriteString(line)
			cur.body.WriteByte('\n')
		}
	}
	flush(len(lines))

	return drafts, nil
}

// fileExt returns the lowercase extension of a path.
func fileExt(path string) string {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(path[i:])
}

// truncate cuts s to at most n runes, appending "..." if truncated.
func truncate(s string, n int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n-3]) + "..."
}
