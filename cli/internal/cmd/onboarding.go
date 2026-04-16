package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vmarinogg/leo-core/cli/internal/profiles"
)

// OnboardingResult holds the choices the user made during the interactive
// onboarding wizard. All values are the internal identifiers used by Leo
// (e.g. "claude", not "Claude Code").
type OnboardingResult struct {
	Runtime        string // "claude", "cursor", "windsurf"
	Language       string // "en", "pt", "es"
	Mode           string // "verbose", "concise", "caveman"
	DefaultProfile string // "general-manager", "ceo", "cto", etc.
	Autonomy       string // "autonomous", "balanced", "supervised"
	CoreSource     string // path to leo-core clone, or "" if skipped
}

// scannerWrapper wraps a bufio.Scanner so we can pass it through the ask*
// helpers without exposing bufio.Scanner's concrete type in the test helper
// signature.
type scannerWrapper struct {
	s *bufio.Scanner
}

func newScannerWrapper(r io.Reader) *scannerWrapper {
	return &scannerWrapper{s: bufio.NewScanner(r)}
}

// readLine reads one line of text. Returns "" on EOF (user pressed Enter on
// an empty line or input ended).
func (sw *scannerWrapper) readLine() string {
	if sw.s.Scan() {
		return strings.TrimSpace(sw.s.Text())
	}
	return ""
}

// separator prints a visual divider to w.
func separator(w io.Writer) {
	fmt.Fprintln(w, "───────────────────────────────────────────")
}

// runOnboarding executes the interactive wizard and returns the chosen config.
// r is the source of user input (os.Stdin in production, strings.Reader in tests).
// w is the destination for wizard output (os.Stdout in production, bytes.Buffer in tests).
// cwd is used for runtime auto-detection.
func runOnboarding(r io.Reader, w io.Writer, cwd string) (OnboardingResult, error) {
	scanner := newScannerWrapper(r)

	// ── Step 1: Welcome ──────────────────────────────────────────────────────
	separator(w)
	fmt.Fprintln(w, "  🦁 Welcome to Leo — Living Ecosystem Orchestrator")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Leo gives your AI coding assistant persistent memory,")
	fmt.Fprintln(w, "  specialist profiles, and structured knowledge management.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Let's set up your project. This takes about 30 seconds.")
	separator(w)
	fmt.Fprintln(w)

	// ── Step 2: Runtime ───────────────────────────────────────────────────────
	rt, err := askRuntime(scanner, w, cwd)
	if err != nil {
		return OnboardingResult{}, err
	}

	// ── Step 3: Language ──────────────────────────────────────────────────────
	lang, err := askLanguage(scanner, w)
	if err != nil {
		return OnboardingResult{}, err
	}

	// ── Step 4: Mode ─────────────────────────────────────────────────────────
	mode, err := askMode(scanner, w)
	if err != nil {
		return OnboardingResult{}, err
	}

	// ── Step 5: Profile ───────────────────────────────────────────────────────
	profile, err := askProfile(scanner, w)
	if err != nil {
		return OnboardingResult{}, err
	}

	// ── Step 6: Autonomy ──────────────────────────────────────────────────────
	autonomy, err := askAutonomy(scanner, w)
	if err != nil {
		return OnboardingResult{}, err
	}

	// ── Step 7: Core Source ───────────────────────────────────────────────────
	coreSource, err := askCoreSource(scanner, w)
	if err != nil {
		return OnboardingResult{}, err
	}

	// ── Step 8: Summary + Confirm ─────────────────────────────────────────────
	fmt.Fprintln(w)
	separator(w)
	fmt.Fprintln(w, "  Here's your configuration:")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "    Runtime:   %s\n", runtimeLabel(rt))
	fmt.Fprintf(w, "    Language:  %s\n", languageLabel(lang))
	fmt.Fprintf(w, "    Mode:      %s\n", modeLabel(mode))
	fmt.Fprintf(w, "    Profile:   %s\n", profileLabel(profile))
	fmt.Fprintf(w, "    Autonomy:  %s\n", autonomyLabel(autonomy))
	fmt.Fprintln(w)
	fmt.Fprint(w, "  Create .leo/ with these settings? [Y/n]: ")

	answer := scanner.readLine()
	separator(w)

	if answer != "" && strings.ToLower(answer) != "y" {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted by user")
	}

	return OnboardingResult{
		Runtime:        rt,
		Language:       lang,
		Mode:           mode,
		DefaultProfile: profile,
		Autonomy:       autonomy,
		CoreSource:     coreSource,
	}, nil
}

// askRuntime prompts for AI runtime selection. It auto-detects .claude/ or
// .cursor/ in cwd and adjusts the default accordingly.
func askRuntime(scanner *scannerWrapper, w io.Writer, cwd string) (string, error) {
	defaultRT, detected := detectRuntime(cwd)

	fmt.Fprintln(w, "  Which AI Assistant do you use?")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  [1] Claude Code  (recommended)")
	fmt.Fprintln(w, "  [2] Cursor       (coming soon)")
	fmt.Fprintln(w, "  [3] Windsurf     (coming soon)")
	fmt.Fprintln(w)

	if detected {
		fmt.Fprintf(w, "  Detected: %s\n", runtimeLabel(defaultRT))
	}

	for {
		fmt.Fprint(w, "  → Choice [1]: ")
		line := scanner.readLine()
		if line == "" {
			return "claude", nil
		}
		switch line {
		case "1":
			return "claude", nil
		case "2":
			fmt.Fprintln(w, "  Cursor support is coming soon. Please select another option.")
			continue
		case "3":
			fmt.Fprintln(w, "  Windsurf support is coming soon. Please select another option.")
			continue
		default:
			fmt.Fprintln(w, "  Invalid choice. Please enter 1.")
		}
	}
}

// askLanguage prompts for language selection.
func askLanguage(scanner *scannerWrapper, w io.Writer) (string, error) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  What output language should Leo use?")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  [1] English")
	fmt.Fprintln(w, "  [2] Português")
	fmt.Fprintln(w, "  [3] Español")
	fmt.Fprintln(w)

	for {
		fmt.Fprint(w, "  → Choice [1]: ")
		line := scanner.readLine()
		if line == "" {
			return "en", nil
		}
		switch line {
		case "1":
			return "en", nil
		case "2":
			return "pt", nil
		case "3":
			return "es", nil
		default:
			fmt.Fprintln(w, "  Invalid choice. Please enter 1, 2, or 3.")
		}
	}
}

// askMode prompts for the communication mode.
func askMode(scanner *scannerWrapper, w io.Writer) (string, error) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  How should Leo communicate?")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  [1] Verbose  — detailed explanations and reasoning")
	fmt.Fprintln(w, "  [2] Concise  — short and direct (recommended)")
	fmt.Fprintln(w, "  [3] Caveman  — minimal tokens, maximum signal")
	fmt.Fprintln(w)

	for {
		fmt.Fprint(w, "  → Choice [2]: ")
		line := scanner.readLine()
		if line == "" {
			return "concise", nil
		}
		switch line {
		case "1":
			return "verbose", nil
		case "2":
			return "concise", nil
		case "3":
			return "caveman", nil
		default:
			fmt.Fprintln(w, "  Invalid choice. Please enter 1, 2, or 3.")
		}
	}
}

// askProfile prompts for the default profile. Only user-scoped profiles are shown.
func askProfile(scanner *scannerWrapper, w io.Writer) (string, error) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Choose your default profile:")
	fmt.Fprintln(w)

	allProfiles := profiles.DefaultProfiles()

	// Collect and sort user-scoped profile names.
	names := make([]string, 0)
	for name, p := range allProfiles {
		if p.Scope == "user" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	// Find default index (general-manager).
	defaultIdx := 1
	for i, name := range names {
		if name == "general-manager" {
			defaultIdx = i + 1
			break
		}
	}

	// Display options.
	for i, name := range names {
		p := allProfiles[name]
		fmt.Fprintf(w, "  [%d] %s — %s\n", i+1, p.Name, p.Description)
	}
	fmt.Fprintln(w)

	maxChoice := len(names)
	for {
		fmt.Fprintf(w, "  → Choice [%d]: ", defaultIdx)
		line := scanner.readLine()
		if line == "" {
			return "general-manager", nil
		}
		// Parse the number.
		var choice int
		if _, err := fmt.Sscanf(line, "%d", &choice); err != nil || choice < 1 || choice > maxChoice {
			fmt.Fprintf(w, "  Invalid choice. Please enter 1 to %d.\n", maxChoice)
			continue
		}
		return names[choice-1], nil
	}
}

// askAutonomy prompts for the autonomy level.
func askAutonomy(scanner *scannerWrapper, w io.Writer) (string, error) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  How much autonomy should Leo have?")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  [1] Autonomous — acts independently, asks only when critical")
	fmt.Fprintln(w, "  [2] Balanced   — proposes plans, confirms before major changes")
	fmt.Fprintln(w, "  [3] Supervised — confirms every significant action")
	fmt.Fprintln(w)

	for {
		fmt.Fprint(w, "  → Choice [2]: ")
		line := scanner.readLine()
		if line == "" {
			return "balanced", nil
		}
		switch line {
		case "1":
			return "autonomous", nil
		case "2":
			return "balanced", nil
		case "3":
			return "supervised", nil
		default:
			fmt.Fprintln(w, "  Invalid choice. Please enter 1, 2, or 3.")
		}
	}
}

// askCoreSource prompts the user for the path to their leo-core clone.
// Returns "" if the user skips, or the expanded path if valid.
func askCoreSource(scanner *scannerWrapper, w io.Writer) (string, error) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Path to your leo-core clone (for updates)?")
	fmt.Fprintln(w, "  (Leave blank to skip — configure later in .leo/config.yaml)")
	fmt.Fprintln(w)
	fmt.Fprint(w, "  → Path [skip]: ")
	line := scanner.readLine()
	if line == "" {
		return "", nil
	}
	expanded := expandTilde(line)
	kbDocsDir := filepath.Join(expanded, ".leo", "kb", "docs")
	if _, err := os.Stat(kbDocsDir); err != nil {
		return "", fmt.Errorf("not a valid leo-core: %s not found", kbDocsDir)
	}
	return expanded, nil
}

// expandTilde replaces a leading "~/" with the user's home directory.
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// detectRuntime checks if a known runtime directory exists in cwd.
// Returns the detected runtime identifier and a boolean indicating whether
// detection actually occurred. Only returns runtimes that are currently
// supported (claude). Falls back to "claude" when nothing is found.
func detectRuntime(cwd string) (rt string, detected bool) {
	if isDir(filepath.Join(cwd, ".claude")) {
		return "claude", true
	}
	// Cursor/Windsurf detection disabled until supported.
	return "claude", false
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func runtimeToNum(rt string) int {
	switch rt {
	case "cursor":
		return 2
	case "windsurf":
		return 3
	case "other":
		return 4
	default:
		return 1
	}
}

func runtimeLabel(rt string) string {
	switch rt {
	case "claude":
		return "Claude Code"
	case "cursor":
		return "Cursor"
	case "windsurf":
		return "Windsurf"
	default:
		return "Other"
	}
}

func languageLabel(lang string) string {
	switch lang {
	case "pt":
		return "Português"
	case "es":
		return "Español"
	default:
		return "English"
	}
}

func profileLabel(profile string) string {
	allProfiles := profiles.DefaultProfiles()
	if p, ok := allProfiles[profile]; ok {
		return p.Name
	}
	return profile
}

func modeLabel(mode string) string {
	switch mode {
	case "verbose":
		return "Verbose"
	case "caveman":
		return "Caveman"
	default:
		return "Concise"
	}
}

func autonomyLabel(autonomy string) string {
	switch autonomy {
	case "autonomous":
		return "Autonomous"
	case "supervised":
		return "Supervised"
	default:
		return "Balanced"
	}
}
