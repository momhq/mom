package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// indexLinkRe matches inline markdown links whose target is a vault content
// file under any known layout directory (ICM: reference, contracts, episodes;
// legacy: topics, timeline, summaries, dev-log). The target path is captured
// so it can be checked against the set of files the garden pass produced.
var indexLinkRe = regexp.MustCompile(`\[[^\]]*\]\((\.?/?(?:reference|contracts|episodes|topics|timeline|summaries|dev-log)/[^)\s#]+\.md)[^)]*\)`)

const (
	// gardenModel is the stronger default model for garden passes. Dedupe
	// and reorganization need more capability than the cheap fold model.
	gardenModel = "claude-sonnet-4-6"
)

// GardenResult is the outcome of a garden pass: the reorganized file set
// and INDEX, plus before/after counts for the command summary.
type GardenResult struct {
	// Result carries the reorganized Files + Index + (deterministic) ClaudeBlock.
	Result FoldResult
	// FilesBefore is the count of existing vault files before gardening.
	FilesBefore int
	// FilesAfter is the count of files the garden pass produced.
	FilesAfter int
	// Model is the model actually used.
	Model string
}

// Garden reorganizes the EXISTING vault for coherence WITHOUT inventing new
// memories: it merges near-duplicate topic files, prunes empties, fixes
// cross-links, and tightens INDEX. It REQUIRES claude — there is no
// deterministic garden (dedupe needs an LLM). On any failure it warns and
// returns an error; the caller must leave the vault unchanged.
func Garden(ctx context.Context, projectRoot, projectID, model string, warn func(string)) (GardenResult, error) {
	if warn == nil {
		warn = func(string) {}
	}
	if strings.TrimSpace(model) == "" {
		model = gardenModel
	}

	existing, err := LoadExisting(projectRoot)
	if err != nil {
		return GardenResult{}, fmt.Errorf("load existing vault: %w", err)
	}
	if len(existing) == 0 {
		return GardenResult{}, fmt.Errorf("no vault files to garden; run `mom vault fold` first")
	}

	if _, err := exec.LookPath("claude"); err != nil {
		warn("claude not on PATH; garden requires it — vault left unchanged")
		return GardenResult{}, fmt.Errorf("claude not on PATH: %w", err)
	}

	prompt := buildGardenPrompt(projectID, existing)

	// disableAllHooks: same rationale as the fold invoker — the user's
	// MOM-installed Stop/SessionEnd sweep hooks must not run (or fail) inside
	// a synthesis subprocess.
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", model, "--output-format", "json",
		"--settings", `{"disableAllHooks": true}`)
	// Run in a neutral directory so the subprocess does not inherit the
	// target project's CLAUDE.md / skills / MCP as its own context.
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		warn(fmt.Sprintf("garden synthesis failed (%v); vault left unchanged", err))
		return GardenResult{}, fmt.Errorf("claude exit: %w (stderr: %s)", err, truncate(stderr.String(), 200))
	}

	assistantText := extractAssistantText(stdout.String())
	jsonObj := extractJSONObject(assistantText)
	if strings.TrimSpace(jsonObj) == "" {
		warn("garden produced no JSON object; vault left unchanged")
		return GardenResult{}, fmt.Errorf("no JSON object in garden output")
	}

	var out llmOut
	if err := json.Unmarshal([]byte(jsonObj), &out); err != nil {
		warn(fmt.Sprintf("garden output was not valid JSON (%v); vault left unchanged", err))
		return GardenResult{}, fmt.Errorf("parse garden JSON: %w", err)
	}
	if len(out.Files) == 0 && strings.TrimSpace(out.Index) == "" {
		warn("garden returned an empty result; vault left unchanged")
		return GardenResult{}, fmt.Errorf("empty garden result")
	}

	files := map[string]string{}
	for _, f := range out.Files {
		p := strings.TrimSpace(f.Path)
		if p == "" || p == indexFileName {
			continue
		}
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		files[p] = f.Content
	}
	if len(files) == 0 {
		warn("garden returned no usable files; vault left unchanged")
		return GardenResult{}, fmt.Errorf("garden produced no usable files")
	}

	// Safety net: a single garden call can hit the model's output-token
	// limit on a large vault and truncate its `files` array while its INDEX
	// still links to files it never emitted. Strip any INDEX cross-link that
	// points at a topics//timeline/ file not in the returned set, so we never
	// write dangling links (garden rule: keep cross-links valid).
	index := pruneDanglingIndexLinks(out.Index, files, warn)

	// Build a deterministic CLAUDE.md block. Reuse the existing watermark so
	// the pointer offset is accurate; garden does not advance it.
	st, _, _ := LoadFoldState(projectRoot)
	block := buildClaudeBlock(FoldInput{ProjectID: projectID, ToOffset: st.LastOffset})

	res := FoldResult{Files: files, Index: index, ClaudeBlock: block}
	return GardenResult{
		Result:      res,
		FilesBefore: len(existing),
		FilesAfter:  len(files),
		Model:       model,
	}, nil
}

// pruneDanglingIndexLinks rewrites INDEX markdown so any link to a
// topics//timeline/ file absent from the produced set is downgraded to plain
// text (the link is removed, the label kept). This keeps the router valid
// even if a single garden call truncated its file list.
func pruneDanglingIndexLinks(index string, files map[string]string, warn func(string)) string {
	if strings.TrimSpace(index) == "" {
		return index
	}
	have := map[string]bool{}
	for p := range files {
		have[strings.TrimPrefix(strings.TrimPrefix(p, "./"), "/")] = true
	}
	dropped := 0
	out := indexLinkRe.ReplaceAllStringFunc(index, func(m string) string {
		sub := indexLinkRe.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		target := strings.TrimPrefix(strings.TrimPrefix(sub[1], "./"), "/")
		if have[target] {
			return m
		}
		// Dangling link: keep the human-readable label, drop the link.
		dropped++
		label := m
		if i := strings.Index(m, "]("); i > 0 {
			label = m[1:i] // text between [ and ](
		}
		return label
	})
	if dropped > 0 {
		warn(fmt.Sprintf("garden INDEX referenced %d file(s) not in the produced set (likely output truncation); demoted those links to plain text", dropped))
	}
	return out
}

// buildGardenPrompt assembles the reorganization prompt. It instructs the
// model to merge/dedupe/relink WITHOUT inventing memories and to preserve
// provenance citations verbatim.
func buildGardenPrompt(projectID string, existing map[string]string) string {
	var b strings.Builder
	b.WriteString("You are the synthesis engine for MOM, an open-source developer memory tool. The user runs `mom vault garden` to tidy their OWN project's notes — a set of markdown files they captured themselves. This is a legitimate documentation-maintenance task the user explicitly invoked; there is no hidden party. Your job is to REORGANIZE these existing markdown notes for coherence WITHOUT inventing anything, and return the cleaned-up files so the CLI can write them back to the user's disk.\n\n")
	b.WriteString("RULES (follow exactly):\n")
	b.WriteString("1. Do NOT invent, add, or fabricate any memory or fact not already present in the files below.\n")
	b.WriteString("2. MERGE files that cover the same topic into one well-structured file (e.g. \"blog-writing-rules.md\" + \"blog-writing-standards.md\" → one file); remove redundancy.\n")
	b.WriteString("3. PRESERVE provenance citations VERBATIM (e.g. \"Session `abc123`, turns N–M\", \"session `abc123`\"). Never drop, alter, or fabricate them.\n")
	b.WriteString("4. DROP empty or near-empty files.\n")
	b.WriteString("5. Keep cross-links valid: use relative markdown links that point at files that still exist after your reorganization.\n")
	b.WriteString("6. Rewrite INDEX.md as a TIGHT router: a two-column table `| Read this | What it covers |`. Column 1 MUST be a clickable relative markdown link whose text is the path, e.g. ``[`topics/voice.md`](topics/voice.md)`` — never a bare path. Column 2 is a short description of what the file covers (its subject/title), not a slug echo. Every link MUST point at a file you include in \"files\" — never link a file you did not emit.\n")
	b.WriteString("7. Never restate or paraphrase the project's CLAUDE.md / human-owned canon. The CLAUDE.md pointer block is generated automatically; set \"claude_block\" to an empty string.\n")
	b.WriteString("8. CONSOLIDATE aggressively: prefer a SMALL number of larger, well-structured files (aim for roughly half the input file count or fewer). You must emit the COMPLETE content of every file you list — do not leave any referenced file out. Keep total output compact so nothing is truncated.\n\n")
	b.WriteString("OUTPUT FORMAT: The `mom` CLI parses your reply as JSON, so respond with a single JSON object and nothing else (no surrounding prose, no markdown code fence). Use EXACTLY this shape — `content` holds the full reorganized markdown for each file:\n")
	b.WriteString(`{"files":[{"path":"topics/voice.md","content":"# Topic: Voice\n..."}],"index":"# MOM Vault Index\n...","claude_block":""}` + "\n\n")
	fmt.Fprintf(&b, "PROJECT: %s\n", projectID)
	b.WriteString("\n=== EXISTING VAULT FILES (reorganize these) ===\n")
	keys := make([]string, 0, len(existing))
	for k := range existing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "\n--- FILE: %s ---\n%s\n", k, existing[k])
	}
	return b.String()
}
