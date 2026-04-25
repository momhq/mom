package cmd

import (
	"os"
	"time"

	"github.com/momhq/mom/cli/internal/ux"
	"github.com/spf13/cobra"
)

var demoCmd = &cobra.Command{
	Use:    "demo",
	Short:  "Preview MOM CLI design system patterns",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := ux.NewPrinter(os.Stdout)

		// Banner
		p.Banner()
		p.Blank()

		// Intro text
		p.Text("MOM gives your AI coding assistant persistent memory")
		p.Text("and structured knowledge management.")
		p.Blank()
		p.Text("Let's set up your project. This takes about 30 seconds.")
		p.Blank()

		// Diamond section — multi-select
		p.Diamond("Which AI Assistants do you want to enable?")
		p.Blank()
		p.Selected("Claude Code")
		p.Selected("Codex")
		p.Unselected("Windsurf")
		p.Blank()

		// Diamond section — single select with descriptions
		p.Diamond("Communication mode")
		p.Blank()
		p.Chevron("Concise — direct, no filler, grammar intact (recommended)")
		p.Indent("Efficient — telegraphic, fragments OK, max token savings")
		p.Indent("Default — no instructions, runtime decides")
		p.Blank()

		// Diamond section — path select
		p.Diamond("Where should MOM be installed?")
		p.Blank()
		p.Chevron("~/Github  (org — spans all repos here) — recommended")
		p.Indent("~/Github/mom  (this project only)")
		p.Indent("Custom path...")
		p.Blank()

		// Diamond section — yes/no
		p.Diamond("Scan existing content to seed your memory?")
		p.Blank()
		p.Chevron("Yes — scan this repo")
		p.Indent("No — start empty")
		p.Blank()

		// Configuration summary
		p.Bold("Configuration Summary")
		p.Blank()
		w := 12
		p.KeyValue("Runtimes", "Claude Code, Codex", w)
		p.KeyValue("Language", "English", w)
		p.KeyValue("Mode", "Concise", w)
		p.KeyValue("Scope", "org (~/Github)", w)
		p.Blank()

		// Confirmation
		p.Diamond("Create .mom/ with these settings? " + p.SuccessText("Yes"))
		p.Blank()

		// Spinner demo
		sp := ux.NewSpinner(os.Stderr)
		sp.Start("Scanning project structure")
		time.Sleep(1 * time.Second)
		sp.Stop()

		sp2 := ux.NewSpinner(os.Stderr)
		sp2.Start("Writing memory structure")
		time.Sleep(800 * time.Millisecond)
		sp2.Stop()

		sp3 := ux.NewSpinner(os.Stderr)
		sp3.Start("Generating runtime context files")
		time.Sleep(800 * time.Millisecond)
		sp3.Stop()

		p.Blank()

		// Checkmarks
		p.Check(".mom/ structure created")
		p.Check("CLAUDE.md")
		p.Check(".claude/settings.json (MCP registered)")
		p.Check(".claude/settings.json (hooks registered)")
		p.Check(".codex/instructions.md")
		p.Check(".gitignore updated (4 entries added)")
		p.Blank()

		// Final line
		p.Textf("MOM is ready. Run %s to check health.", p.HighlightCmd("mom status"))
		p.Blank()

		// Validation patterns
		p.Bold("--- Validation Patterns ---")
		p.Blank()
		p.Check("passing check")
		p.Fail("failing check")
		p.Warn("warning check")
		p.Blank()

		// Step done pattern
		p.StepDone("Scanning project structure")
		p.StepDone("Writing memory structure")
		p.StepDone("Generating runtime context files")
		p.Blank()

		return nil
	},
}
