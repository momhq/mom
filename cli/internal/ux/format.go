package ux

import (
	"fmt"
	"io"
	"strings"
)

// Printer handles styled output with TTY awareness.
// When tty is false all styling is stripped — plain text with same structure.
type Printer struct {
	W   io.Writer
	tty bool
}

// NewPrinter creates a Printer. Styling is enabled only when w is a TTY.
func NewPrinter(w io.Writer) *Printer {
	return &Printer{W: w, tty: IsTTY(w)}
}

// --- low-level helpers ---

func (p *Printer) style(s string, fn func(...string) string) string {
	if p.tty {
		return fn(s)
	}
	return s
}

// --- text rendering ---

// Text prints body text in Paper color.
func (p *Printer) Text(s string) {
	fmt.Fprintln(p.W, p.style(s, TextStyle.Render))
}

// Textf prints formatted body text in Paper color.
func (p *Printer) Textf(format string, a ...any) {
	p.Text(fmt.Sprintf(format, a...))
}

// Bold prints bold Paper text.
func (p *Printer) Bold(s string) {
	fmt.Fprintln(p.W, p.style(s, BoldStyle.Render))
}

// Muted prints muted/secondary text.
func (p *Printer) Muted(s string) {
	fmt.Fprintln(p.W, p.style(s, MutedStyle.Render))
}

// Blank prints an empty line.
func (p *Printer) Blank() {
	fmt.Fprintln(p.W)
}

// --- inline styling (return string, no newline) ---

// HighlightValue returns a string rendered in Archive color (for key values).
func (p *Printer) HighlightValue(s string) string {
	return p.style(s, ArchiveStyle.Render)
}

// HighlightCmd returns a string rendered in Archive color + bold (for commands).
func (p *Printer) HighlightCmd(s string) string {
	if p.tty {
		return ArchiveStyle.Bold(true).Render(s)
	}
	return s
}

// SuccessText returns a string rendered in Success color.
func (p *Printer) SuccessText(s string) string {
	return p.style(s, SuccessStyle.Render)
}

// ErrorText returns a string rendered in Error color.
func (p *Printer) ErrorText(s string) string {
	return p.style(s, ErrorStyle.Render)
}

// WarningText returns a string rendered in Warning color.
func (p *Printer) WarningText(s string) string {
	return p.style(s, WarningStyle.Render)
}

// --- bullets and indicators ---

// Diamond prints a filled diamond (◆) in Signal + bold text.
// Used for section headings / step phases.
func (p *Printer) Diamond(s string) {
	bullet := p.style(DiamondFilled, SignalStyle.Render)
	label := p.style(s, BoldStyle.Render)
	fmt.Fprintf(p.W, "%s %s\n", bullet, label)
}

// Check prints a checkmark (✔) in Success + Paper text.
func (p *Printer) Check(s string) {
	bullet := p.style(Checkmark, SuccessStyle.Render)
	label := p.style(s, TextStyle.Render)
	fmt.Fprintf(p.W, "%s %s\n", bullet, label)
}

// Checkf prints a formatted checkmark line.
func (p *Printer) Checkf(format string, a ...any) {
	p.Check(fmt.Sprintf(format, a...))
}

// Fail prints a cross (✗) in Error + Paper text.
func (p *Printer) Fail(s string) {
	bullet := p.style(Cross, ErrorStyle.Render)
	label := p.style(s, TextStyle.Render)
	fmt.Fprintf(p.W, "%s %s\n", bullet, label)
}

// Failf prints a formatted fail line.
func (p *Printer) Failf(format string, a ...any) {
	p.Fail(fmt.Sprintf(format, a...))
}

// Warn prints a warning sign (⚠) in Warning + Paper text.
func (p *Printer) Warn(s string) {
	bullet := p.style(WarningSign, WarningStyle.Render)
	label := p.style(s, TextStyle.Render)
	fmt.Fprintf(p.W, "%s %s\n", bullet, label)
}

// Warnf prints a formatted warning line.
func (p *Printer) Warnf(format string, a ...any) {
	p.Warn(fmt.Sprintf(format, a...))
}

// Chevron prints a chevron (›) in Archive + Paper text.
// Used for list items under a Diamond section.
func (p *Printer) Chevron(s string) {
	bullet := p.style(Chevron, ArchiveStyle.Render)
	label := p.style(s, TextStyle.Render)
	fmt.Fprintf(p.W, "  %s %s\n", bullet, label)
}

// Selected prints a filled bullet (●) in Archive + Paper text.
func (p *Printer) Selected(s string) {
	bullet := p.style(BulletFilled, ArchiveStyle.Render)
	label := p.style(s, TextStyle.Render)
	fmt.Fprintf(p.W, "    %s %s\n", bullet, label)
}

// Unselected prints an empty bullet (○) in Paper + Paper text.
func (p *Printer) Unselected(s string) {
	label := p.style(s, TextStyle.Render)
	fmt.Fprintf(p.W, "    %s %s\n", BulletEmpty, label)
}

// Indent prints indented text (for descriptions under list items).
func (p *Printer) Indent(s string) {
	label := p.style(s, MutedStyle.Render)
	fmt.Fprintf(p.W, "      %s\n", label)
}

// --- step progress ---

// StepDone prints a diamond step that completed: ◆ label... done
func (p *Printer) StepDone(label string) {
	bullet := p.style(DiamondFilled, SignalStyle.Render)
	text := p.style(label+"...", TextStyle.Render)
	done := p.style("done", SuccessStyle.Render)
	fmt.Fprintf(p.W, "%s %s %s\n", bullet, text, done)
}

// --- key-value pairs ---

// KeyValue prints a label-value pair with consistent alignment.
// Label in Paper, value in Archive.
func (p *Printer) KeyValue(label string, value string, width int) {
	l := p.style(label, TextStyle.Render)
	v := p.style(value, ArchiveStyle.Render)
	pad := ""
	if width > len(label) {
		pad = strings.Repeat(" ", width-len(label))
	}
	fmt.Fprintf(p.W, "%s%s  %s\n", l, pad, v)
}

// --- banner ---

// Banner prints the MOM ASCII art banner in Archive color.
func (p *Printer) Banner() {
	art := []string{
		`██ ██       ██ ██   ██ ██ ██ ██ ██ ██   ██ ██       ██ ██`,
		`██ ██ ██ ██ ██ ██   ██ ██       ██ ██   ██ ██ ██ ██ ██ ██`,
		`██    ██ ██    ██   ██ ██       ██ ██   ██    ██ ██    ██`,
		`██             ██   ██ ██ ██ ██ ██ ██   ██             ██`,
	}
	for _, line := range art {
		fmt.Fprintln(p.W, p.style(line, ArchiveStyle.Render))
	}
	fmt.Fprintln(p.W, p.style("Memory Oriented Machine", ArchiveStyle.Render))
}
