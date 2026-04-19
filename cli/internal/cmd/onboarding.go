package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
)

// OnboardingResult holds the choices the user made during the interactive
// onboarding wizard. All values are the internal identifiers used by Leo.
type OnboardingResult struct {
	Runtimes   []string // ["claude", "codex", "cline"]
	Language   string   // "en", "pt", "es"
	Mode       string   // "verbose", "concise", "normal", "caveman"
	Autonomy   string   // "autonomous", "balanced", "supervised"
	CoreSource string   // path to leo-core clone, or "" if skipped
}

// runOnboarding executes the interactive wizard and returns the chosen config.
// r is the source of user input (os.Stdin in production, strings.Reader in tests).
// w is the destination for wizard output (os.Stdout in production, bytes.Buffer in tests).
// cwd is used for runtime auto-detection.
func runOnboarding(r io.Reader, w io.Writer, cwd string) (OnboardingResult, error) {
	accessible := !isTerminalReader(r)

	// ── Prepare runtime options ─────────────────────────────────────────────
	registry := runtime.NewRegistry(cwd)
	allAdapters := registry.All()
	detected := registry.DetectAll()

	detectedSet := make(map[string]bool)
	for _, a := range detected {
		detectedSet[a.Name()] = true
	}
	if len(detectedSet) == 0 {
		detectedSet["claude"] = true
	}

	var runtimeOptions []huh.Option[string]
	for _, a := range allAdapters {
		opt := huh.NewOption(runtimeLabel(a.Name()), a.Name())
		if detectedSet[a.Name()] {
			opt = opt.Selected(true)
		}
		runtimeOptions = append(runtimeOptions, opt)
	}

	// ── Bind variables ──────────────────────────────────────────────────────
	var selectedRuntimes []string
	lang := "en"
	mode := "concise"
	autonomy := "balanced"
	coreSource := ""

	// ── Build the form ──────────────────────────────────────────────────────
	form := huh.NewForm(
		// Group 1: Welcome
		huh.NewGroup(
			huh.NewNote().
				Title("Welcome to L.E.O.").
				Description(
					"Living Ecosystem Orchestrator\n\n"+
						"L.E.O. gives your AI coding assistant persistent memory\n"+
						"and structured knowledge management.\n\n"+
						"Let's set up your project. This takes about 30 seconds.",
				),
		),

		// Group 2: Runtimes
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which AI Assistants do you want to enable?").
				Options(runtimeOptions...).
				Height(len(runtimeOptions)+2).
				Value(&selectedRuntimes).
				Validate(func(selected []string) error {
					if len(selected) == 0 {
						return fmt.Errorf("select at least one runtime")
					}
					return nil
				}),
		),

		// Group 3: Language + Communication mode
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What output language should L.E.O. use?").
				Options(
					huh.NewOption("English", "en"),
					huh.NewOption("Português", "pt"),
					huh.NewOption("Español", "es"),
				).
				Value(&lang),

			huh.NewSelect[string]().
				Title("Communication mode").
				Options(
					huh.NewOption("Concise — short and direct (recommended)", "concise"),
					huh.NewOption("Normal — standard prose", "normal"),
					huh.NewOption("Verbose — detailed explanations", "verbose"),
					huh.NewOption("Caveman — minimal tokens, maximum signal", "caveman"),
				).
				Value(&mode),
		),

		// Group 4: Autonomy
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How much autonomy should L.E.O. have?").
				Options(
					huh.NewOption("Autonomous — acts independently, asks only when critical", "autonomous"),
					huh.NewOption("Balanced — proposes plans, confirms before major changes", "balanced"),
					huh.NewOption("Supervised — confirms every significant action", "supervised"),
				).
				Value(&autonomy),
		),

		// Group 5: Core Source
		huh.NewGroup(
			huh.NewInput().
				Title("Path to your leo-core clone (for updates)").
				Description("Leave blank to skip — configure later in .leo/config.yaml").
				Value(&coreSource),
		),
	).WithAccessible(accessible).
		WithInput(r).
		WithOutput(w).
		WithTheme(huh.ThemeFunc(huh.ThemeDracula))

	if err := form.Run(); err != nil {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted: %w", err)
	}

	// Validate and expand core source path.
	if coreSource != "" {
		expanded := expandTilde(coreSource)
		kbDocsDir := filepath.Join(expanded, ".leo", "kb", "docs")
		if _, err := os.Stat(kbDocsDir); err != nil {
			return OnboardingResult{}, fmt.Errorf("not a valid leo-core: %s not found", kbDocsDir)
		}
		coreSource = expanded
	}

	// ── Summary + Confirm ───────────────────────────────────────────────────
	summaryText := fmt.Sprintf(
		"  Runtimes:  %s\n  Language:  %s\n  Mode:      %s\n  Autonomy:  %s",
		runtimesLabel(selectedRuntimes),
		languageLabel(lang),
		modeLabel(mode),
		autonomyLabel(autonomy),
	)

	confirmed := true
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Configuration Summary").
				Description(summaryText),
			huh.NewConfirm().
				Title("Create .leo/ with these settings?").
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	).WithAccessible(accessible).
		WithInput(r).
		WithOutput(w).
		WithTheme(huh.ThemeFunc(huh.ThemeDracula))

	if err := confirmForm.Run(); err != nil {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted: %w", err)
	}

	if !confirmed {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted by user")
	}

	return OnboardingResult{
		Runtimes:   selectedRuntimes,
		Language:   lang,
		Mode:       mode,
		Autonomy:   autonomy,
		CoreSource: coreSource,
	}, nil
}

// isTerminalReader returns true if r is connected to a terminal.
func isTerminalReader(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(f.Fd())
	}
	return false
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

func runtimeLabel(rt string) string {
	switch rt {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "cline":
		return "Cline"
	case "cursor":
		return "Cursor"
	case "windsurf":
		return "Windsurf"
	default:
		return rt
	}
}

func runtimesLabel(rts []string) string {
	labels := make([]string, len(rts))
	for i, rt := range rts {
		labels[i] = runtimeLabel(rt)
	}
	return strings.Join(labels, ", ")
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

func modeLabel(mode string) string {
	switch mode {
	case "verbose":
		return "Verbose"
	case "caveman":
		return "Caveman"
	case "normal":
		return "Normal"
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
