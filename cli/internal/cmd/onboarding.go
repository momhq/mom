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
	Language   string   // always "en" — language selection removed in v0.9
	Mode       string   // "verbose", "concise", "normal", "caveman"
	CoreSource string   // path to leo-core clone, or "" if skipped
	// InstallDir is the directory where .leo/ should be created.
	// Defaults to cwd (current project). Set to a parent for multi-repo installs.
	InstallDir string
	// ScopeLabel is the value written to config.yaml scope: field.
	// Defaults to "repo".
	ScopeLabel string
	// BootstrapChoice is the user's answer to the bootstrap question.
	// Values: "full" | "repo" | "skip". Empty means skip (non-interactive path).
	BootstrapChoice string
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
	// Language is fixed to "en"; the prompt was removed in v0.9.
	lang := "en"
	mode := "concise"
	coreSource := ""
	bootstrapChoice := "skip" // default: skip

	// Scope installation choice: "cwd" (repo), a detected parent dir, or "custom".
	// scopeChoice maps to an install directory; scopeLabel tracks the config value.
	scopeChoice := "cwd" // default: current directory
	customScopePath := ""

	// Detect common parent dirs to offer as suggestions.
	home, _ := os.UserHomeDir()
	parentSuggestions := detectParentDirs(cwd, home)
	scopeOptions := buildScopeOptions(cwd, parentSuggestions)

	// ── Build the form ──────────────────────────────────────────────────────
	form := huh.NewForm(
		// Group 1: Welcome
		huh.NewGroup(
			huh.NewNote().
				Title("Welcome to MOM").
				Description(
					"Memory Oriented Machine\n\n"+
						"MOM gives your AI coding assistant persistent memory\n"+
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

		// Group 3: Communication mode
		huh.NewGroup(
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

		// Group 4: Scope / install location
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Where should MOM be installed?").
				Description("Installing in a parent folder lets MOM span all repos beneath it.").
				Options(scopeOptions...).
				Value(&scopeChoice),
		),

		// Group 5: Core Source
		huh.NewGroup(
			huh.NewInput().
				Title("Path to your leo-core clone (for updates)").
				Description("Leave blank to skip — configure later in .leo/config.yaml").
				Value(&coreSource),
		),

		// Group 6: Bootstrap
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Scan existing content to seed your memory?").
				Description(
					"Reads code, markdown, and commit messages to create initial memories.\n"+
						"You can skip and let memory build from sessions only.",
				).
				Options(
					huh.NewOption("Yes — scan this repo", "repo"),
					huh.NewOption("No — start empty", "skip"),
				).
				Value(&bootstrapChoice),
		),
	).WithAccessible(accessible).
		WithInput(r).
		WithOutput(w).
		WithTheme(huh.ThemeFunc(huh.ThemeDracula))

	if err := form.Run(); err != nil {
		return OnboardingResult{}, fmt.Errorf("onboarding aborted: %w", err)
	}

	// Resolve scope choice into an install directory and scope label.
	installDir, scopeLabel := resolveScopeChoice(scopeChoice, customScopePath, cwd, parentSuggestions)

	// Validate and expand core source path — accept both new (memory) and legacy (kb/docs) layouts.
	if coreSource != "" {
		expanded := expandTilde(coreSource)
		memoryDir := filepath.Join(expanded, ".leo", "memory")
		if _, err := os.Stat(memoryDir); err != nil {
			// Fall back to legacy layout.
			legacyDir := filepath.Join(expanded, ".leo", "kb", "docs")
			if _, err := os.Stat(legacyDir); err != nil {
				return OnboardingResult{}, fmt.Errorf("not a valid leo-core: %s not found", memoryDir)
			}
		}
		coreSource = expanded
	}

	// ── Summary + Confirm ───────────────────────────────────────────────────
	scopeDisplay := installDir
	if scopeDisplay == cwd {
		scopeDisplay = "current directory (repo)"
	}
	summaryText := fmt.Sprintf(
		"  Runtimes:  %s\n  Language:  %s\n  Mode:      %s\n  Scope:     %s (%s)",
		runtimesLabel(selectedRuntimes),
		languageLabel(lang),
		modeLabel(mode),
		scopeLabel,
		scopeDisplay,
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
		Runtimes:        selectedRuntimes,
		Language:        lang,
		Mode:            mode,
		CoreSource:      coreSource,
		InstallDir:      installDir,
		ScopeLabel:      scopeLabel,
		BootstrapChoice: bootstrapChoice,
	}, nil
}

// ParentScope holds role information about a parent directory detected above cwd.
type ParentScope struct {
	Path       string   // absolute path
	Label      string   // "user", "org", "repo"
	HasGit     bool     // true if the directory itself contains .git/
	ChildRepos []string // paths of immediate children that contain .git/
}

// containsGitRepos returns true if dir has at least one immediate child
// directory that itself contains a .git/ subdirectory.
func containsGitRepos(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		gitPath := filepath.Join(dir, e.Name(), ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// childGitRepos returns paths of all immediate children of dir that contain .git/.
func childGitRepos(dir string) []string {
	var result []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(dir, e.Name())
		gitPath := filepath.Join(child, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			result = append(result, child)
		}
	}
	return result
}

// detectParentDirs returns up to 2 parent directories above cwd with role labels.
// Role assignment:
//   - directory whose immediate children contain .git/ → "org"
//   - directory with .git/ in itself → "repo"
//   - otherwise → "workspace"
//
// Stops walking at home (home itself is excluded; it gets "user" role only if
// it appears as a direct parent and containsGitRepos).
func detectParentDirs(cwd, home string) []ParentScope {
	var parents []ParentScope
	dir := filepath.Dir(cwd)
	for dir != cwd {
		if dir == home || dir == filepath.Dir(home) {
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}

		hasGit := false
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			hasGit = true
		}
		children := childGitRepos(dir)

		var label string
		switch {
		case len(children) > 0:
			label = "org"
		case hasGit:
			label = "repo"
		default:
			label = "workspace"
		}

		parents = append(parents, ParentScope{
			Path:       dir,
			Label:      label,
			HasGit:     hasGit,
			ChildRepos: children,
		})

		if len(parents) >= 2 {
			break
		}
		dir = next
	}
	return parents
}

// discoverUninitializedChildRepos returns paths of immediate children of dir
// that have .git/ but do not have .leo/.
func discoverUninitializedChildRepos(dir string) []string {
	var result []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(dir, e.Name())
		gitPath := filepath.Join(child, ".git")
		leoPath := filepath.Join(child, ".leo")
		gitInfo, gitErr := os.Stat(gitPath)
		_, leoErr := os.Stat(leoPath)
		if gitErr == nil && gitInfo.IsDir() && os.IsNotExist(leoErr) {
			result = append(result, child)
		}
	}
	return result
}

// cwdScopeRole returns the semantic scope label for cwd by inspecting its
// children. If cwd contains git repos (directly or via org sub-folders), it
// gets "user" or "org"; otherwise "repo".
func cwdScopeRole(cwd string) string {
	// Direct children with .git/ → cwd is at least an org folder.
	if containsGitRepos(cwd) {
		return "user"
	}
	// Grandchildren: check if any immediate child dir contains git repos
	// (i.e. cwd is a root above org folders).
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return "repo"
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if containsGitRepos(filepath.Join(cwd, e.Name())) {
			return "user"
		}
	}
	return "repo"
}

// buildScopeOptions builds the huh Select options for the scope question.
// It considers both parent directories (above cwd) and the role of cwd itself.
func buildScopeOptions(cwd string, parents []ParentScope) []huh.Option[string] {
	home, _ := os.UserHomeDir()
	var opts []huh.Option[string]

	for i, p := range parents {
		display := p.Path
		if strings.HasPrefix(p.Path, home) {
			display = "~" + p.Path[len(home):]
		}
		label := fmt.Sprintf("%s  (%s — spans all repos here)", display, p.Label)
		if i == 0 {
			label += " — recommended"
		}
		opts = append(opts, huh.NewOption(label, "parent:"+p.Path))
	}

	cwdDisplay := cwd
	if strings.HasPrefix(cwd, home) {
		cwdDisplay = "~" + cwd[len(home):]
	}

	// Evaluate cwd's own role: if it contains repos, show it as user/org scope.
	cwdRole := cwdScopeRole(cwd)
	if cwdRole == "user" || cwdRole == "org" {
		cwdLabel := fmt.Sprintf("%s  (%s — spans all repos here)", cwdDisplay, cwdRole)
		if len(parents) == 0 {
			cwdLabel += " — recommended"
		}
		opts = append(opts, huh.NewOption(cwdLabel, "cwd"))
	} else {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%s  (this project only)", cwdDisplay), "cwd"))
	}

	opts = append(opts, huh.NewOption("Custom path…", "custom"))
	return opts
}

// resolveScopeChoice converts the user's scopeChoice into an install directory
// and a scope label for config.yaml. Labels are derived from the ParentScope
// role, not array position.
func resolveScopeChoice(choice, customPath, cwd string, parents []ParentScope) (installDir, scopeLabel string) {
	switch {
	case choice == "cwd":
		role := cwdScopeRole(cwd)
		return cwd, role
	case choice == "custom":
		expanded := expandTilde(customPath)
		if expanded == "" {
			return cwd, "repo"
		}
		return expanded, "custom"
	case strings.HasPrefix(choice, "parent:"):
		dir := strings.TrimPrefix(choice, "parent:")
		for _, p := range parents {
			if p.Path == dir {
				return dir, p.Label
			}
		}
		// Fallback: unknown parent, default to "user".
		return dir, "user"
	default:
		return cwd, "repo"
	}
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
