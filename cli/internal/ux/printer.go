// Package ux provides lightweight terminal output helpers for MOM commands.
package ux

import (
	"fmt"
	"io"
)

// Printer writes formatted output to a writer.
// It provides convenience methods for common output patterns.
type Printer struct {
	w io.Writer
}

// NewPrinter creates a Printer that writes to w.
func NewPrinter(w io.Writer) *Printer {
	return &Printer{w: w}
}

// Checkf writes a success-prefixed formatted line.
func (p *Printer) Checkf(format string, args ...any) {
	fmt.Fprintf(p.w, "✓ "+format+"\n", args...)
}

// Warnf writes a warning-prefixed formatted line.
func (p *Printer) Warnf(format string, args ...any) {
	fmt.Fprintf(p.w, "! "+format+"\n", args...)
}

// Infof writes an info-prefixed formatted line.
func (p *Printer) Infof(format string, args ...any) {
	fmt.Fprintf(p.w, "  "+format+"\n", args...)
}

// Textf writes a plain formatted line.
func (p *Printer) Textf(format string, args ...any) {
	fmt.Fprintf(p.w, format+"\n", args...)
}

// Muted writes a dimmed-style plain string line.
func (p *Printer) Muted(s string) {
	fmt.Fprintln(p.w, s)
}

// Blank writes an empty line.
func (p *Printer) Blank() {
	fmt.Fprintln(p.w)
}
