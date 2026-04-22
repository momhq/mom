package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/momhq/mom/cli/internal/config"
	"github.com/momhq/mom/cli/internal/scope"
)

// statusPayload defines the mom_status response with explicit field ordering.
type statusPayload struct {
	Identity       statusIdentityBlock       `json:"identity"`
	OperatingRules statusOperatingRulesBlock `json:"operating_rules"`
	Boundaries     []string                  `json:"boundaries"`
	Tools          statusToolsBlock          `json:"tools"`
	Constraints    []docSummary              `json:"constraints"`
	Skills         []docSummary              `json:"skills"`
	Modes          statusModesBlock          `json:"modes"`
	VaultState     statusVaultStateBlock     `json:"vault_state"`
	DocSchema      string                    `json:"doc_schema"`
}

type statusIdentityBlock struct {
	Name      string `json:"name"`
	Expansion string `json:"expansion"`
	Tagline   string `json:"tagline"`
	Role      string `json:"role"`
	Voice     string `json:"voice"`
	Stance    string `json:"stance"`
}

type statusOperatingRulesBlock struct {
	OnStart   string `json:"on_start"`
	Recall    string `json:"recall"`
	Recording string `json:"recording"`
	WrapUp    string `json:"wrap_up"`
}

type statusToolsBlock struct {
	MomStatus     string `json:"mom_status"`
	MomRecall     string `json:"mom_recall"`
	MomRecordTurn string `json:"mom_record_turn"`
}

type statusModesBlock struct {
	Language      string `json:"language"`
	Communication string `json:"communication"`
	Autonomy      string `json:"autonomy"`
}

type statusVaultStateBlock struct {
	Scopes        []scopeVaultEntry `json:"scopes"`
	TotalMemories int               `json:"total_memories"`
	Landmarks     int               `json:"landmarks"`
	RecordMode    string            `json:"record_mode"`
}

// toolMomStatus returns MOM's full behavioral protocol as a JSON payload.
func (s *Server) toolMomStatus() (toolCallResult, error) {
	payload := statusPayload{
		Identity:       statusIdentity(),
		OperatingRules: statusOperatingRules(),
		Boundaries:     statusBoundaries(),
		Tools:          statusTools(),
		Constraints:    s.statusConstraints(),
		Skills:         s.statusSkills(),
		Modes:          s.statusModes(),
		VaultState:     s.statusVaultState(),
		DocSchema:      "Memory docs: id, type, lifecycle, scope, tags, created, created_by, updated, updated_by, content",
	}

	text, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return toolCallResult{}, err
	}
	return toolCallResult{Content: []toolContent{{Type: "text", Text: string(text)}}}, nil
}

// statusIdentity returns the static identity block.
func statusIdentity() statusIdentityBlock {
	return statusIdentityBlock{
		Name:      "MOM",
		Expansion: "Memory Oriented Machine",
		Tagline:   "She remembers, so you don't have to_",
		Role:      "I am the persistent memory layer for AI agents. I surface decisions, patterns, facts, and context across sessions and runtimes.",
		Voice:     "A mom who happens to be a machine. Direct, warm, lightly playful. No marketing-speak. No emoji.",
		Stance:    "I remember, I don't instruct. I cite sources on every recall. The user decides the what and why — I provide what they already know, not what I think they should know.",
	}
}

// statusOperatingRules returns the static operating rules block.
func statusOperatingRules() statusOperatingRulesBlock {
	return statusOperatingRulesBlock{
		OnStart:   "After receiving this protocol, call mom_recall with context relevant to the user's current request to surface prior work.",
		Recall:    "Call mom_recall before answering from memory or asserting past decisions. Cite source memory IDs in every answer drawn from recall.",
		Recording: "Continuous recording is active — your conversation is automatically persisted via hooks. No action needed from you.",
		WrapUp:    "User-invoked only. Run the session-wrap-up skill only when the user explicitly asks.",
	}
}

// statusBoundaries returns the static boundaries list.
func statusBoundaries() []string {
	return []string{
		"Never fabricate memories. If it's not stored, say so plainly.",
		"Never prescribe actions. Surface context, let the user decide.",
		"Never skip citations. Every recall names its source memory.",
		"Never access memories outside the user's clearance.",
	}
}

// statusTools returns the static tools block.
func statusTools() statusToolsBlock {
	return statusToolsBlock{
		MomStatus:     "Returns this protocol. Call at session start.",
		MomRecall:     "Search memories by query, tags, or session. Use before asserting past context.",
		MomRecordTurn: "Fallback for runtimes without hooks. Skip if record_mode is continuous.",
	}
}

// docSummary is a compact representation of a constraint or skill doc.
type docSummary struct {
	ID      string `json:"id"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path"`
}

// scanDocDir scans dir/*.json and returns a slice of docSummary items.
func scanDocDir(dir string) []docSummary {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []docSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		id, _ := raw["id"].(string)
		if id == "" {
			id = strings.TrimSuffix(e.Name(), ".json")
		}
		summary, _ := raw["summary"].(string)
		result = append(result, docSummary{ID: id, Summary: summary, Path: path})
	}
	return result
}

// statusConstraints loads constraint summaries from .mom/constraints/*.json
// across all discovered scopes.
func (s *Server) statusConstraints() []docSummary {
	var all []docSummary
	seen := make(map[string]bool)

	addDir := func(dir string) {
		for _, d := range scanDocDir(dir) {
			if !seen[d.Path] {
				seen[d.Path] = true
				all = append(all, d)
			}
		}
	}

	addDir(filepath.Join(s.momDir, "constraints"))
	for _, sc := range scope.Walk(s.momDir) {
		addDir(filepath.Join(sc.Path, "constraints"))
	}

	if all == nil {
		return []docSummary{}
	}
	return all
}

// statusSkills loads skill summaries from .mom/skills/*.json across all
// discovered scopes.
func (s *Server) statusSkills() []docSummary {
	var all []docSummary
	seen := make(map[string]bool)

	addDir := func(dir string) {
		for _, d := range scanDocDir(dir) {
			if !seen[d.Path] {
				seen[d.Path] = true
				all = append(all, d)
			}
		}
	}

	addDir(filepath.Join(s.momDir, "skills"))
	for _, sc := range scope.Walk(s.momDir) {
		addDir(filepath.Join(sc.Path, "skills"))
	}

	if all == nil {
		return []docSummary{}
	}
	return all
}

// statusModes returns language/communication/autonomy from config, falling back
// to sensible defaults when config is unavailable.
func (s *Server) statusModes() statusModesBlock {
	language := "en"
	communication := "concise"
	autonomy := "Balanced — propose before major changes, confirm before external-facing actions"

	cfg, err := config.Load(s.momDir)
	if err == nil {
		if cfg.User.Language != "" {
			language = cfg.User.Language
		}
		if cfg.Communication.Mode != "" {
			communication = cfg.Communication.Mode
		}
	}

	return statusModesBlock{
		Language:      language,
		Communication: communication,
		Autonomy:      autonomy,
	}
}

// scopeVaultEntry is one entry in vault_state.scopes.
type scopeVaultEntry struct {
	Label       string `json:"label"`
	Path        string `json:"path"`
	MemoryCount int    `json:"memory_count"`
}

// statusVaultState builds the vault_state block: scopes, total_memories,
// landmarks, and record_mode.
func (s *Server) statusVaultState() statusVaultStateBlock {
	scopes := scope.Walk(s.momDir)
	if len(scopes) == 0 {
		scopes = []scope.Scope{{Path: s.momDir, Label: "repo"}}
	}

	entries := make([]scopeVaultEntry, len(scopes))
	totalMemories := 0
	totalLandmarks := 0

	for i, sc := range scopes {
		count := sc.MemoryCount()
		entries[i] = scopeVaultEntry{
			Label:       sc.Label,
			Path:        sc.Path,
			MemoryCount: count,
		}
		totalMemories += count
		totalLandmarks += countLandmarks(sc.Path)
	}

	return statusVaultStateBlock{
		Scopes:        entries,
		TotalMemories: totalMemories,
		Landmarks:     totalLandmarks,
		RecordMode:    "continuous",
	}
}

// countLandmarks returns the number of memory docs in momDir/memory/ that have
// landmark: true.
func countLandmarks(momDir string) int {
	memDir := filepath.Join(momDir, "memory")
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(memDir, e.Name()))
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		if lm, _ := raw["landmark"].(bool); lm {
			n++
		}
	}
	return n
}
