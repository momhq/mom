package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
	"github.com/momhq/mom/ingress/harness"
	"github.com/momhq/mom/shared/ux"
)

// OnboardingResult holds the choices the user made during the interactive
// onboarding wizard. All values are the internal identifiers used by MOM.
type OnboardingResult struct {
	Harnesses  []string // ["claude", "codex", "pi"]
	Language   string   // always "en" — language selection removed
	Mode       string   // "default", "concise", "efficient"
	CoreSource string   // path to mom clone, or "" if skipped
	// InstallDir is the current project directory registered with the global watcher.
	InstallDir string
	// ScopeLabel is retained for legacy config compatibility. Global init writes repo.
	ScopeLabel string
	// BindCwd is true when the user opted to bind the current directory as a
	// project so capture starts immediately.
	BindCwd bool
	// ProjectId is the id to bind the current directory to when BindCwd is true.
	ProjectId string
}

// runOnboarding executes the interactive wizard and returns the chosen config.
// r is the source of user input (os.Stdin in production, strings.Reader in tests).
// w is the destination for wizard output (os.Stdout in production, bytes.Buffer in tests).
// cwd is used for harness auto-detection.
func runOnboarding(r io.Reader, w io.Writer, cwd string) (OnboardingResult, error) {
	accessible := !isTerminalReader(r)

	// ── Prepare harness options ─────────────────────────────────────────────
	registry := harness.NewRegistry(cwd)
	allAdapters := registry.All()
	detected := registry.DetectAll()

	detectedSet := make(map[string]bool)
	for _, a := range detected {
		detectedSet[a.Name()] = true
	}
	if len(detectedSet) == 0 {
		detectedSet["claude"] = true
	}

	var harnessOptions []huh.Option[string]
	for _, a := range allAdapters {
		label := harnessLabel(a.Name())
		if detectedSet[a.Name()] {
			label += " (detected)"
		}
		opt := huh.NewOption(label, a.Name())
		if detectedSet[a.Name()] {
			opt = opt.Selected(true)
		}
		harnessOptions = append(harnessOptions, opt)
	}

	// ── Bind variables ──────────────────────────────────────────────────────
	var selectedHarnesses []string
	// Language is fixed to "en"; the prompt was removed.
	lang := "en"
	mode := "concise"

	// The central vault and harness integrations are global. cwd is only recorded
	// as the active project for watcher metadata.
	installDir := cwd
	scopeLabel := "repo"

	// Capture is privacy-gated: it only runs in directories bound to a project.
	// Offer to bind the current directory so capture starts on first run, but
	// only when it is a sensible project root (not $HOME or the central vault).
	bindable := isBindableDir(cwd)
	bindCwd := true
	projectId := deriveProjectId(cwd)

	// ── Build the form ──────────────────────────────────────────────────────
	groups := []*huh.Group{
		// Group 1: Welcome
		huh.NewGroup(
			huh.NewNote().
				Title(
					" ███╗   ███╗  ██████╗  ███╗   ███╗\n" +
						" ████╗ ████║ ██╔═══██╗ ████╗ ████║\n" +
						" ██╔████╔██║ ██║   ██║ ██╔████╔██║\n" +
						" ██║╚██╔╝██║ ██║   ██║ ██║╚██╔╝██║\n" +
						" ██║ ╚═╝ ██║ ╚██████╔╝ ██║ ╚═╝ ██║\n" +
						" ╚═╝     ╚═╝  ╚═════╝  ╚═╝     ╚═╝\n" +
						" Memory Oriented Machine",
				).
				Description(
					"\nMOM gives your AI coding assistant persistent memory\n" +
						"and structured knowledge management.\n\n" +
						"Setting up MOM takes about 30 seconds. Let's start.",
				),
		),

		// Group 2: Harnesses
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which AI Assistants do you want to enable?").
				Options(harnessOptions...).
				Height(len(harnessOptions) + 2).
				Value(&selectedHarnesses).
				Validate(func(selected []string) error {
					if len(selected) == 0 {
						return fmt.Errorf("select at least one harness")
					}
					return nil
				}),
		),

		// Group 3: Communication mode
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Communication mode").
				Options(
					huh.NewOption("Concise — direct, no filler, grammar intact (recommended)", "concise"),
					huh.NewOption("Efficient — telegraphic, fragments OK, max token savings", "efficient"),
					huh.NewOption("Default — no instructions, harness decides", "default"),
				).
				Value(&mode),
		),
	}

	// Group 4 (conditional): ask whether to bind this directory so capture
	// starts here. The id is derived from the directory name; the user can
	// rebind to a different id later with `mom project bind --id`.
	if bindable {
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Start capturing this directory as project %q?", projectId)).
				Description("Writes .mom-project.yaml here so the watcher records sessions from this repo.\nCapture only runs in bound directories. Change or remove it anytime with mom project bind.").
				Affirmative("Yes").
				Negative("Not now").
				Value(&bindCwd),
		))
	}

	form := huh.NewForm(groups...).WithAccessible(accessible).
		WithInput(r).
		WithOutput(w).
		WithTheme(huh.ThemeFunc(ux.ThemeMOM))

	if err := form.Run(); err != nil {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted: %w", err)
	}

	// ── Summary + Confirm ───────────────────────────────────────────────────
	captureLine := "  Capture:   bind later (mom project bind --id <id>)"
	if bindable && bindCwd {
		captureLine = fmt.Sprintf("  Capture:   this directory, project %q", projectId)
	}
	summaryText := fmt.Sprintf(
		"  Harnesses: %s\n  Language:  %s\n  Mode:      %s\n%s",
		harnessesLabel(selectedHarnesses),
		languageLabel(lang),
		modeLabel(mode),
		captureLine,
	)

	confirmed := true
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Configuration Summary").
				Description(summaryText),
			huh.NewConfirm().
				Title("Install MOM globally with these settings?").
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	).WithAccessible(accessible).
		WithInput(r).
		WithOutput(w).
		WithTheme(huh.ThemeFunc(ux.ThemeMOM))

	if err := confirmForm.Run(); err != nil {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted: %w", err)
	}

	if !confirmed {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted by user")
	}

	return OnboardingResult{
		Harnesses:  selectedHarnesses,
		Language:   lang,
		Mode:       mode,
		CoreSource: "",
		InstallDir: installDir,
		ScopeLabel: scopeLabel,
		BindCwd:    bindable && bindCwd,
		ProjectId:  projectId,
	}, nil
}

// isTerminalReader returns true if r is connected to a terminal.
func isTerminalReader(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(f.Fd())
	}
	return false
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func harnessLabel(rt string) string {
	switch rt {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "cursor":
		return "Cursor"
	case "pi":
		return "Pi"
	case "droid":
		return "Factory Droid"
	default:
		return rt
	}
}

func harnessesLabel(rts []string) string {
	labels := make([]string, len(rts))
	for i, rt := range rts {
		labels[i] = harnessLabel(rt)
	}
	return strings.Join(labels, ", ")
}

func languageLabel(_ string) string {
	return "English"
}

func modeLabel(mode string) string {
	switch mode {
	case "concise":
		return "Concise"
	case "efficient":
		return "Efficient"
	default:
		return "Default"
	}
}
