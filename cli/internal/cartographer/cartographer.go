// Package cartographer scans existing code, docs, and commits to seed the KB
// with initial memories. It is the bootstrap pass for new Leo installations.
package cartographer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Confidence constants mirror the KB schema values.
const (
	ConfidenceExtracted = "EXTRACTED"
	ConfidenceInferred  = "INFERRED"
	ConfidenceAmbiguous = "AMBIGUOUS"
)

// TriggerEvent is written into every draft's provenance.
const TriggerEvent = "bootstrap.scan"

// Extractor is the interface every source-of-memories implements.
type Extractor interface {
	// Name returns a human-readable identifier for this extractor.
	Name() string
	// Extract returns memory drafts from a single source unit.
	Extract(ctx context.Context, source Source) ([]Draft, error)
	// Matches returns whether this extractor handles the given path.
	Matches(path string) bool
}

// Source abstracts where content comes from (file, commit, manifest).
type Source struct {
	Path      string
	Content   []byte
	Extension string
	IsCommit  bool
	CommitSHA string
}

// Draft is a proposed memory before it gets schema-validated and written.
type Draft struct {
	Type       string // decision | fact | pattern | learning
	Summary    string
	Tags       []string
	Confidence string // EXTRACTED | INFERRED | AMBIGUOUS
	Content    map[string]any
	Provenance ProvenanceMeta
}

// ProvenanceMeta records where a draft came from.
type ProvenanceMeta struct {
	SourceFile   string
	SourceLines  string // "42-48" or "42"
	SourceHash   string // SHA256 of the source content
	TriggerEvent string // always "bootstrap.scan"
	CommitSHA    string
}

// Config controls a Cartographer scan pass.
type Config struct {
	// CommitDepth is how many recent commits to inspect (default 200).
	CommitDepth int
	// MaxFileSizeMB skips files larger than this (default 2).
	MaxFileSizeMB int64
	// SkipPatterns is a list of glob patterns to skip (e.g. "vendor/**").
	SkipPatterns []string
	// Extensions is the list of file extensions to scan for text extractors.
	Extensions []string
	// Refresh forces re-scanning all files, ignoring the cache.
	Refresh bool
	// DryRun shows what would be written without persisting.
	DryRun bool
	// ScopeDir is the .leo/ directory to write memories into.
	ScopeDir string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		CommitDepth:   200,
		MaxFileSizeMB: 2,
		SkipPatterns: []string{
			"node_modules/**",
			"vendor/**",
			"**/.git/**",
			"dist/**",
			"build/**",
			".leo/**",
		},
		Extensions: []string{
			".md", ".mdx", ".txt", ".rst",
		},
	}
}

// Result summarises a completed scan pass.
type Result struct {
	RootDir   string
	StartedAt time.Time
	EndedAt   time.Time
	Drafts    []Draft

	// Breakdown by extractor name.
	ByExtractor map[string]ExtractorResult
}

// ExtractorResult holds per-extractor counts.
type ExtractorResult struct {
	Name      string
	Count     int
	Extracted int
	Inferred  int
	Ambiguous int
}

// Duration returns the wall-clock time taken.
func (r *Result) Duration() time.Duration {
	return r.EndedAt.Sub(r.StartedAt)
}

// Cartographer orchestrates a scan pass over a directory tree.
type Cartographer struct {
	cfg        Config
	extractors []Extractor
	cache      *Cache
}

// New creates a Cartographer with the given config and default extractors.
// Call AddExtractor to append additional extractors.
func New(cfg Config) *Cartographer {
	c := &Cartographer{
		cfg:   cfg,
		cache: NewCache(cfg.ScopeDir),
	}
	// Register default extractors in the prescribed order.
	c.extractors = []Extractor{
		NewMarkdownExtractor(),
		NewDependencyManifestExtractor(),
		NewCommitLogExtractor(cfg.CommitDepth),
		NewTodoFixmeExtractor(),
		NewTreeSitterASTExtractor(),
	}
	return c
}

// AddExtractor appends an extractor to the pipeline.
func (c *Cartographer) AddExtractor(e Extractor) {
	c.extractors = append(c.extractors, e)
}

// Scan walks rootDir and runs all extractors, returning the combined result.
func (c *Cartographer) Scan(ctx context.Context, rootDir string) (*Result, error) {
	result := &Result{
		RootDir:     rootDir,
		StartedAt:   time.Now(),
		ByExtractor: make(map[string]ExtractorResult),
	}

	// Collect file paths.
	paths, err := c.collectFiles(rootDir)
	if err != nil {
		return nil, fmt.Errorf("collecting files: %w", err)
	}

	// Run file-based extractors.
	var mu sync.Mutex
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		content, err := c.readFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		src := Source{
			Path:      path,
			Content:   content,
			Extension: strings.ToLower(filepath.Ext(path)),
		}
		srcHash := hashBytes(content)

		// Check cache.
		if !c.cfg.Refresh && c.cache != nil {
			if entry, ok := c.cache.Get(path); ok && entry.SHA256 == srcHash {
				continue // not changed
			}
		}

		for _, ext := range c.extractors {
			if ext.Name() == "commits" || !ext.Matches(path) {
				continue
			}
			drafts, err := ext.Extract(ctx, src)
			if err != nil {
				continue
			}

			mu.Lock()
			result.Drafts = append(result.Drafts, drafts...)
			addToResult(result, ext.Name(), drafts)
			mu.Unlock()
		}

		// Update cache entry.
		if c.cache != nil {
			c.cache.Set(path, CacheEntry{
				SHA256:        srcHash,
				LastScannedAt: time.Now().UTC().Format(time.RFC3339),
				DraftCount:    0, // updated below
			})
		}
	}

	// Run commit extractor (not file-based).
	for _, ext := range c.extractors {
		if ext.Name() != "commits" {
			continue
		}
		src := Source{Path: rootDir, IsCommit: true}
		drafts, err := ext.Extract(ctx, src)
		if err == nil {
			result.Drafts = append(result.Drafts, drafts...)
			addToResult(result, ext.Name(), drafts)
		}
	}

	result.EndedAt = time.Now()

	// Persist cache.
	if c.cache != nil && !c.cfg.DryRun {
		_ = c.cache.Save()
	}

	return result, nil
}

// collectFiles walks rootDir and returns all files that pass size and pattern filters.
func (c *Cartographer) collectFiles(rootDir string) ([]string, error) {
	maxBytes := c.cfg.MaxFileSizeMB * 1024 * 1024
	if maxBytes == 0 {
		maxBytes = 2 * 1024 * 1024
	}

	var paths []string

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Get path relative to rootDir for pattern matching.
		rel, _ := filepath.Rel(rootDir, path)

		// Skip directories matching skip patterns.
		if d.IsDir() {
			if matchesAnyPattern(rel, c.cfg.SkipPatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip symlinks.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		// Skip if matches skip patterns.
		if matchesAnyPattern(rel, c.cfg.SkipPatterns) {
			return nil
		}

		// Skip oversized files.
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > maxBytes {
			return nil
		}

		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	return paths, nil
}

// readFile reads a file and returns its contents.
func (c *Cartographer) readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// addToResult accumulates draft counts into the result's ByExtractor map.
func addToResult(result *Result, extName string, drafts []Draft) {
	er := result.ByExtractor[extName]
	er.Name = extName
	for _, d := range drafts {
		er.Count++
		switch d.Confidence {
		case ConfidenceExtracted:
			er.Extracted++
		case ConfidenceInferred:
			er.Inferred++
		case ConfidenceAmbiguous:
			er.Ambiguous++
		}
	}
	result.ByExtractor[extName] = er
}

// matchesAnyPattern returns true if path matches any of the provided glob patterns.
func matchesAnyPattern(path string, patterns []string) bool {
	// Normalise path separators.
	path = filepath.ToSlash(path)
	for _, pattern := range patterns {
		if globMatch(pattern, path) {
			return true
		}
	}
	return false
}

// globMatch is a simple double-star glob matcher.
// Supports **, *, ?, and character classes are not needed here.
func globMatch(pattern, name string) bool {
	// Use a recursive approach for ** support.
	return globMatchRec(pattern, name)
}

func globMatchRec(pattern, name string) bool {
	if pattern == "" {
		return name == ""
	}

	if pattern == "**" {
		return true
	}

	if strings.HasPrefix(pattern, "**/") {
		rest := pattern[3:]
		// Match any number of path segments.
		if globMatchRec(rest, name) {
			return true
		}
		// Consume one path segment from name.
		idx := strings.Index(name, "/")
		if idx >= 0 {
			return globMatchRec(pattern, name[idx+1:])
		}
		return false
	}

	if strings.HasSuffix(pattern, "/**") {
		prefix := pattern[:len(pattern)-3]
		return name == prefix || strings.HasPrefix(name, prefix+"/")
	}

	// Split on first / and match segment.
	pi := strings.Index(pattern, "/")
	ni := strings.Index(name, "/")

	if pi < 0 && ni < 0 {
		return segMatch(pattern, name)
	}
	if pi < 0 || ni < 0 {
		return false
	}
	return segMatch(pattern[:pi], name[:ni]) && globMatchRec(pattern[pi+1:], name[ni+1:])
}

// segMatch matches a single path segment with * and ? support.
func segMatch(pattern, segment string) bool {
	if pattern == "*" {
		return true
	}

	// Simple character matching with *.
	pi, si := 0, 0
	starIdx := -1
	match := 0

	for si < len(segment) {
		if pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == segment[si]) {
			pi++
			si++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starIdx = pi
			match = si
			pi++
		} else if starIdx >= 0 {
			pi = starIdx + 1
			match++
			si = match
		} else {
			return false
		}
	}

	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// hashBytes returns the SHA256 hex digest of the given bytes.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// DraftHash returns the SHA256 hex digest of the given string.
// Exported for use by CLI command layer when generating draft IDs.
func DraftHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// lineRange formats a line range as "start-end" or "start".
func lineRange(start, end int) string {
	if end <= start {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

// linesOf splits b into lines (without trailing newline per element).
func linesOf(b []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
