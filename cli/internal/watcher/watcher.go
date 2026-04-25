package watcher

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/momhq/mom/cli/internal/recorder"
)

// Config holds watcher configuration (mirrors .mom/config.yaml watcher block).
type Config struct {
	// TranscriptDir is the directory to watch (e.g. ~/.claude/projects/).
	// Tilde expansion is performed automatically.
	TranscriptDir string
	// MomDir is the path to .mom/ where raw/ and cursor files are written.
	MomDir string
	// Adapter parses runtime-specific JSONL lines.
	Adapter Adapter
	// DebounceMs is how long to wait after a Write event before reading.
	// Defaults to 300ms if zero.
	DebounceMs int
}

// Watcher watches a Claude Code transcript directory and ingests new entries
// into .mom/raw/ using cursor-based incremental reads.
type Watcher struct {
	cfg     Config
	fw      *fsnotify.Watcher
	mu      sync.Mutex
	timers  map[string]*time.Timer // debounce timers keyed by file path
	rawDir  string
	logFile string
}

// New creates a Watcher. Call Run to start watching.
func New(cfg Config) (*Watcher, error) {
	if cfg.DebounceMs == 0 {
		cfg.DebounceMs = 300
	}

	dir, err := expandTilde(cfg.TranscriptDir)
	if err != nil {
		return nil, fmt.Errorf("expanding transcript dir: %w", err)
	}
	cfg.TranscriptDir = dir

	rawDir := filepath.Join(cfg.MomDir, "raw")
	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return nil, fmt.Errorf("creating raw dir: %w", err)
	}

	logsDir := filepath.Join(cfg.MomDir, "logs")
	_ = os.MkdirAll(logsDir, 0755)

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Watcher{
		cfg:     cfg,
		fw:      fw,
		timers:  make(map[string]*time.Timer),
		rawDir:  rawDir,
		logFile: filepath.Join(logsDir, "watch.log"),
	}, nil
}

// Run starts the watcher loop. It blocks until ctx-equivalent stop is called.
// Returns when the watcher is stopped or encounters an unrecoverable error.
// Call Stop to terminate.
func (w *Watcher) Run() error {
	// Watch the transcript directory recursively — Claude Code creates per-project
	// subdirectories under ~/.claude/projects/.
	if err := w.addDir(w.cfg.TranscriptDir); err != nil {
		return fmt.Errorf("watching %s: %w", w.cfg.TranscriptDir, err)
	}

	// Process any existing files on startup (catch up on offline turns).
	w.catchUp()

	w.logf("watcher started on %s", w.cfg.TranscriptDir)

	for {
		select {
		case event, ok := <-w.fw.Events:
			if !ok {
				return nil // watcher closed
			}
			w.handleEvent(event)

		case err, ok := <-w.fw.Errors:
			if !ok {
				return nil
			}
			w.logf("fsnotify error: %v", err)
		}
	}
}

// Stop shuts down the underlying fsnotify watcher.
func (w *Watcher) Stop() error {
	return w.fw.Close()
}

// handleEvent dispatches fsnotify events.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// New directory created — watch it (Claude Code creates project dirs).
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			_ = w.addDir(path)
			return
		}
	}

	// Only care about .jsonl files.
	if !strings.HasSuffix(path, ".jsonl") {
		return
	}

	// Skip subagent files (Phase 1 scope: top-level sessions only).
	if strings.Contains(path, "subagents") {
		return
	}

	if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
		w.scheduleRead(path)
	}
}

// scheduleRead debounces rapid writes: resets the timer for the given path.
func (w *Watcher) scheduleRead(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	d := time.Duration(w.cfg.DebounceMs) * time.Millisecond
	if t, ok := w.timers[path]; ok {
		t.Reset(d)
		return
	}
	w.timers[path] = time.AfterFunc(d, func() {
		w.mu.Lock()
		delete(w.timers, path)
		w.mu.Unlock()
		w.ingestFile(path)
	})
}

// catchUp processes all existing .jsonl files in the transcript dir on startup.
func (w *Watcher) catchUp() {
	_ = filepath.WalkDir(w.cfg.TranscriptDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") && !strings.Contains(path, "subagents") {
			w.ingestFile(path)
		}
		return nil
	})
}

// ingestFile reads new lines from the transcript file since the last cursor,
// normalizes them via the adapter, and appends to .mom/raw/.
func (w *Watcher) ingestFile(path string) {
	sessionID := sessionIDFromPath(path)
	cursorFile := filepath.Join(w.rawDir, ".watch-cursor-"+sessionID)

	// Read cursor offset.
	offset := readWatchCursor(cursorFile)

	// Open and seek.
	f, err := os.Open(path)
	if err != nil {
		w.logf("opening %s: %v", path, err)
		return
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			w.logf("seeking %s to %d: %v", path, offset, err)
			return
		}
	}

	// Read new content.
	var entries []recorder.RawEntry
	var bytesRead int64
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	for scanner.Scan() {
		raw := scanner.Bytes()
		bytesRead += int64(len(raw)) + 1 // +1 for newline

		entry, ok := w.cfg.Adapter.ParseLine(raw, sessionID)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		w.logf("scanning %s: %v", path, err)
	}

	if bytesRead == 0 {
		return
	}

	// Write entries to .mom/raw/<YYYY-MM-DD>.jsonl.
	if len(entries) > 0 {
		if err := w.writeEntries(entries); err != nil {
			w.logf("writing entries from %s: %v", path, err)
			return
		}
	}

	// Advance cursor.
	writeWatchCursor(cursorFile, offset+bytesRead)
}

// writeEntries appends normalized entries to today's raw JSONL file.
func (w *Watcher) writeEntries(entries []recorder.RawEntry) error {
	now := time.Now().UTC()
	dailyFile := filepath.Join(w.rawDir, now.Format("2006-01-02")+".jsonl")

	f, err := os.OpenFile(dailyFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening daily file: %w", err)
	}
	defer f.Close()

	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// addDir adds a directory and all its subdirectories to the fsnotify watcher.
func (w *Watcher) addDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if d.IsDir() {
			if werr := w.fw.Add(path); werr != nil {
				w.logf("watching dir %s: %v", path, werr)
			}
		}
		return nil
	})
}

// sessionIDFromPath extracts a session ID from a .jsonl transcript path.
// Claude Code paths: ~/.claude/projects/{project-slug}/{sessionId}.jsonl
// We use the filename stem as the session ID.
func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

// readWatchCursor reads the byte offset stored in the cursor file.
// Returns 0 if the file doesn't exist or is unreadable (fresh start).
func readWatchCursor(cursorFile string) int64 {
	data, err := os.ReadFile(cursorFile)
	if err != nil {
		return 0
	}
	var offset int64
	if _, err := fmt.Sscan(string(data), &offset); err != nil {
		return 0
	}
	return offset
}

// writeWatchCursor persists a byte offset to the cursor file.
func writeWatchCursor(cursorFile string, offset int64) {
	_ = os.WriteFile(cursorFile, []byte(fmt.Sprintf("%d", offset)), 0644)
}

// expandTilde replaces a leading "~" with the user's home directory.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}

// logf appends a timestamped message to the watcher log file, best-effort.
func (w *Watcher) logf(format string, args ...any) {
	f, err := os.OpenFile(w.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s watcher: "+format+"\n", append([]any{ts}, args...)...)
}
