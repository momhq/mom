package projection

import (
	"context"
	"fmt"
)

// fold thresholds: a hierarchy synth promotes to L1 after l1Threshold chunks
// and to L2 after l2Threshold L1 nodes.
const (
	l1Threshold = 5
	l2Threshold = 10
)

// defaultClaudeFoldModel is the model folds pin when the user chose none.
// Fold prompts are small and templated, so the cheapest tier is enough — and
// pinning it keeps folds from burning the user's default-model usage limits.
// Override with --model or the vault.fold_model config key.
const defaultClaudeFoldModel = "haiku"

// NewSynthesizer builds the fold Synthesizer for the named engine. "auto"
// (or "") probes the supported harness CLIs in priority order
// (claude → codex → pi) and uses the first available. model pins the
// synthesis model; empty selects the engine's cheap default (claude) or the
// CLI's own default (codex, pi). Returns the Synthesizer and the resolved
// engine name; warn receives non-fatal synthesis warnings.
//
// This keeps engine selection inside the projection package — callers ask
// for an engine by name and never touch the concrete synthesizer types.
func NewSynthesizer(engine, model string, warn func(string)) (Synthesizer, string, error) {
	if warn == nil {
		warn = func(string) {}
	}
	build := func(inv HarnessInvoker) Synthesizer {
		applyModel(inv, model, warn)
		return NewHierarchySynth(NewLLMSynth(inv, warn), l1Threshold, l2Threshold)
	}

	switch engine {
	case "auto", "":
		for _, inv := range []HarnessInvoker{NewClaudeInvoker(""), NewCodexInvoker(""), NewPiInvoker("")} {
			if inv.IsAvailable() {
				return build(inv), inv.Name(), nil
			}
		}
		return nil, "", fmt.Errorf("no supported harness found (tried claude, codex, pi) — install one or pass --engine explicitly")
	case "claude", "codex", "pi":
		inv := invokerFor(engine)
		if !inv.IsAvailable() {
			return nil, "", fmt.Errorf("%s CLI not found; install %s or use --engine auto", engine, engine)
		}
		return build(inv), engine, nil
	default:
		return nil, "", fmt.Errorf("invalid engine %q (want auto|claude|codex|pi)", engine)
	}
}

// applyModel pins the synthesis model on the invoker. Claude defaults to the
// cheapest tier when no model was chosen; codex passes the model through only
// when explicitly set; pi has no known model flag, so a requested model is
// ignored with a warning.
func applyModel(inv HarnessInvoker, model string, warn func(string)) {
	switch v := inv.(type) {
	case *ClaudeInvoker:
		if model == "" {
			model = defaultClaudeFoldModel
		}
		v.Model = model
	case *CodexInvoker:
		v.Model = model
	default:
		if model != "" {
			warn(fmt.Sprintf("engine %s does not support model pinning; using its CLI default", inv.Name()))
		}
	}
}

func invokerFor(engine string) HarnessInvoker {
	switch engine {
	case "claude":
		return NewClaudeInvoker("")
	case "codex":
		return NewCodexInvoker("")
	case "pi":
		return NewPiInvoker("")
	default:
		return nil
	}
}

// Fold runs one fold pass over in, dispatching to the hierarchical driver
// when the Synthesizer supports multi-level folding and the flat driver
// otherwise. Callers do not need to know the concrete synthesizer type — the
// fold strategy lives entirely behind this seam.
func Fold(ctx context.Context, synth Synthesizer, in FoldInput, chunkSize int) (FoldResult, error) {
	if hs, ok := synth.(*HierarchySynth); ok {
		return FoldHierarchical(ctx, hs, in, chunkSize)
	}
	return FoldAll(ctx, synth, in, chunkSize)
}
