package projection

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Frontmatter is the structured metadata block prepended to every vault file.
//
// Type/Name/Description are the OKF (Open Knowledge Format) concept metadata:
// the agent reads them first — bit by bit — to decide whether to open the file,
// without loading its body. Type is the single required OKF field; it names the
// ICM layer / concept kind (e.g. "identity", "reference", "contract", "dev-log").
type Frontmatter struct {
	ID             string    // 16 hex chars, content-addressed from project + source offsets
	Type           string    // OKF concept type / ICM layer: identity|reference|contract|dev-log|episode|index
	Name           string    // OKF: short human/agent-facing title of the concept
	Description    string    // OKF: one-line description of what the file holds
	Level          int       // 0=episode, 1=topic/timeline, 2=summary
	Kind           string    // legacy: "episode" | "topic" | "timeline" | "summary" | "index"
	Sources        []uint64  // sorted ledger offsets that contributed to this file
	Tags           []string  // topic tags
	TimeRangeStart time.Time // zero → omitted from output
	TimeRangeEnd   time.Time // zero → omitted from output
	FoldedAt       time.Time
	Version        int      // schema version, always 1 for now
	Children       []string // vault-relative paths of contributing files (L1+ only)
}

// RenderFrontmatter serializes fm as a YAML block wrapped in --- delimiters.
// Zero-value fields are omitted to keep output compact.
func RenderFrontmatter(fm Frontmatter) string {
	var b strings.Builder
	b.WriteString("---\n")

	if fm.ID != "" {
		fmt.Fprintf(&b, "id: %s\n", fm.ID)
	}
	// OKF concept metadata first — this is what an agent scans before opening.
	if fm.Type != "" {
		fmt.Fprintf(&b, "type: %s\n", fm.Type)
	}
	if fm.Name != "" {
		fmt.Fprintf(&b, "name: %s\n", yamlScalar(fm.Name))
	}
	if fm.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", yamlScalar(fm.Description))
	}
	fmt.Fprintf(&b, "level: %d\n", fm.Level)
	if fm.Kind != "" {
		fmt.Fprintf(&b, "kind: %s\n", fm.Kind)
	}
	if len(fm.Sources) > 0 {
		fmt.Fprintf(&b, "sources: [%s]\n", renderOffsetRanges(fm.Sources))
	}
	if len(fm.Tags) > 0 {
		fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(fm.Tags, ", "))
	}
	if !fm.TimeRangeStart.IsZero() {
		fmt.Fprintf(&b, "time_range_start: %s\n", fm.TimeRangeStart.UTC().Format(time.RFC3339))
	}
	if !fm.TimeRangeEnd.IsZero() {
		fmt.Fprintf(&b, "time_range_end: %s\n", fm.TimeRangeEnd.UTC().Format(time.RFC3339))
	}
	if !fm.FoldedAt.IsZero() {
		fmt.Fprintf(&b, "folded_at: %s\n", fm.FoldedAt.UTC().Format(time.RFC3339))
	}
	if fm.Version != 0 {
		fmt.Fprintf(&b, "version: %d\n", fm.Version)
	}
	if len(fm.Children) > 0 {
		b.WriteString("children:\n")
		for _, c := range fm.Children {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
	}

	b.WriteString("---\n")
	return b.String()
}

// ParseFrontmatter reads the leading ---...--- block from content.
// Returns the parsed Frontmatter and the body (content after the closing ---).
// If no frontmatter block exists, body == content and fm is zero-value.
// Malformed individual fields are silently skipped.
func ParseFrontmatter(content string) (Frontmatter, string) {
	if !strings.HasPrefix(content, "---\n") {
		return Frontmatter{}, content
	}
	// Find closing ---
	rest := content[4:]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return Frontmatter{}, content
	}
	block := rest[:end]
	body := rest[end+5:] // skip "\n---\n"

	var fm Frontmatter
	var inChildren bool

	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(line, "  - ") && inChildren {
			fm.Children = append(fm.Children, strings.TrimPrefix(line, "  - "))
			continue
		}
		inChildren = false

		// Handle bare keys like "children:" (no value after colon).
		if strings.TrimSpace(line) == "children:" {
			inChildren = true
			continue
		}

		kv := strings.SplitN(line, ": ", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])

		switch key {
		case "id":
			fm.ID = val
		case "type":
			fm.Type = val
		case "name":
			fm.Name = unquoteYAML(val)
		case "description":
			fm.Description = unquoteYAML(val)
		case "level":
			if n, err := strconv.Atoi(val); err == nil {
				fm.Level = n
			}
		case "kind":
			fm.Kind = val
		case "sources":
			val = strings.Trim(val, "[]")
			for _, p := range strings.Split(val, ",") {
				p = strings.TrimSpace(p)
				// A "lo-hi" token is a compressed run of consecutive offsets.
				if lo, hi, ok := strings.Cut(p, "-"); ok {
					l, lerr := strconv.ParseUint(strings.TrimSpace(lo), 10, 64)
					h, herr := strconv.ParseUint(strings.TrimSpace(hi), 10, 64)
					if lerr == nil && herr == nil && h >= l {
						for n := l; n <= h; n++ {
							fm.Sources = append(fm.Sources, n)
						}
					}
					continue
				}
				if n, err := strconv.ParseUint(p, 10, 64); err == nil {
					fm.Sources = append(fm.Sources, n)
				}
			}
		case "tags":
			val = strings.Trim(val, "[]")
			for _, t := range strings.Split(val, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					fm.Tags = append(fm.Tags, t)
				}
			}
		case "time_range_start":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				fm.TimeRangeStart = t
			}
		case "time_range_end":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				fm.TimeRangeEnd = t
			}
		case "folded_at":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				fm.FoldedAt = t
			}
		case "version":
			if n, err := strconv.Atoi(val); err == nil {
				fm.Version = n
			}
		case "children":
			inChildren = true
		}
	}

	return fm, body
}

// PrependFrontmatter renders fm and prepends it to body.
func PrependFrontmatter(fm Frontmatter, body string) string {
	return RenderFrontmatter(fm) + body
}

// renderOffsetRanges serializes sorted offsets with consecutive runs
// compressed to "lo-hi". Concept files can carry thousands of contributing
// offsets — mostly consecutive turn windows — and writing each one out made a
// single frontmatter line several KB long, drowning the actual content when a
// human opens the file. ParseFrontmatter expands the runs back, so chunkID
// (computed from the expanded set) is unaffected.
func renderOffsetRanges(sources []uint64) string {
	var parts []string
	for i := 0; i < len(sources); {
		j := i
		for j+1 < len(sources) && sources[j+1] == sources[j]+1 {
			j++
		}
		if j > i {
			parts = append(parts, fmt.Sprintf("%d-%d", sources[i], sources[j]))
		} else {
			parts = append(parts, strconv.FormatUint(sources[i], 10))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// ensureTitle guarantees a human-readable H1 at the top of a concept body.
// Synthesis prompts forbid headings (terseness), so the title is stamped
// deterministically from the OKF name — without it, a rendered file is a wall
// of frontmatter followed by bare bullets.
func ensureTitle(fm Frontmatter, body string) string {
	trimmed := strings.TrimLeft(body, "\n")
	if fm.Name == "" || strings.HasPrefix(trimmed, "# ") {
		return body
	}
	return "# " + fm.Name + "\n\n" + trimmed
}

// yamlScalar double-quotes a value when it contains characters that would
// break a bare YAML scalar (colon, leading special chars, quotes). Plain
// values pass through unquoted to keep the frontmatter readable.
func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\"'[]{}") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// unquoteYAML reverses yamlScalar for parsing: strips surrounding quotes and
// unescapes embedded quotes.
func unquoteYAML(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`)
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// chunkID derives a content-addressed ID for a vault file from its source offsets.
// Canonical form: "projectID:sorted_off1:sorted_off2:..."
// Returns the first 16 hex chars of the SHA-256 of that string.
func chunkID(projectID string, offsets []uint64) string {
	sorted := make([]uint64, len(offsets))
	copy(sorted, offsets)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	parts := make([]string, len(sorted))
	for i, o := range sorted {
		parts[i] = strconv.FormatUint(o, 10)
	}
	input := projectID + ":" + strings.Join(parts, ":")
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", h[:8])
}
