package projection

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// LintFinding is one walk-test violation surfaced by LintVault.
type LintFinding struct {
	Path   string
	Rule   string
	Detail string
}

// Size budgets per ICM layer (Phase F). Heuristic ceilings, not hard schema
// limits — a fold that drifts past them is a signal the synthesis prompt
// terseness rules slipped, not a corrupt vault.
const (
	maxIdentityBytes = 4 * 1024
	maxConceptBytes  = 8 * 1024
	maxEpisodeBytes  = 2 * 1024
	// routerBulletBudget bounds how many consecutive bullet lines a router
	// file (INDEX.md) may carry before it looks like leaked concept payload
	// rather than a table/link list.
	routerBulletBudget = 10
)

var mdLinkRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// payloadHeadingRe matches section headings that only a concept file (never
// a router) should carry — the synthesis prompts' own allowed-heading lists
// (buildL1SubjectInput, buildDocumentConceptHint, buildL2Input).
var payloadHeadingRe = regexp.MustCompile(`(?m)^##\s+(Decisions|Gotchas|Current state|Process|Frameworks|Mental models|Principles|Techniques|Anti-patterns|Tells|Invariants|Flow|Where detail lives)\b`)

// LintVault walk-tests a project's vault for structural hygiene:
//  1. Reachability — every concept file is reachable within 2 hops of a
//     routing file (root INDEX.md → folder INDEX.md → concept). Episodes are
//     exempt: they are provenance, deliberately unlinked from routing.
//  2. No payload in routers — INDEX.md, */INDEX.md, and the MOM-managed
//     block in harness entry files (CLAUDE.md, AGENTS.md) must carry only
//     headings/tables/links, never concept payload.
//  3. Size budget — identity.md / reference & conventions concepts /
//     episodes each stay under their layer's ceiling.
//
// root is the project root (the same root VaultDir/LoadExisting take).
func LintVault(root string) ([]LintFinding, error) {
	files, err := loadVaultFiles(VaultDir(root))
	if err != nil {
		return nil, err
	}

	var findings []LintFinding
	findings = append(findings, lintReachability(files)...)
	findings = append(findings, lintNoPayloadInRouters(files)...)
	findings = append(findings, lintSizeBudget(files)...)
	findings = append(findings, lintEntryFiles(root)...)

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Rule < findings[j].Rule
	})
	return findings, nil
}

// loadVaultFiles reads every .md file under vaultDir, INDEX.md files
// included (unlike LoadExisting, which is synthesis-input-only and skips
// the router files it regenerates each fold).
func loadVaultFiles(vaultDir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.Walk(vaultDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, rerr := filepath.Rel(vaultDir, path)
		if rerr != nil {
			return rerr
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lint: scan vault: %w", err)
	}
	return out, nil
}

// extractLinks returns the local (non-http) markdown link targets in content.
func extractLinks(content string) []string {
	var out []string
	for _, m := range mdLinkRe.FindAllStringSubmatch(content, -1) {
		target := m[1]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			continue
		}
		out = append(out, target)
	}
	return out
}

// resolveLink joins a link found inside fromPath (vault-relative) against
// fromPath's directory, producing a vault-relative path.
func resolveLink(fromPath, link string) string {
	dir := filepath.Dir(fromPath)
	if dir == "." {
		return filepath.ToSlash(filepath.Clean(link))
	}
	return filepath.ToSlash(filepath.Clean(filepath.Join(dir, link)))
}

// lintReachability flags files not reachable within 2 hops of the root
// INDEX.md — an orphan a fold produced but no router links to.
func lintReachability(files map[string]string) []LintFinding {
	rootIdx, ok := files[indexFileName]
	if !ok {
		return nil
	}
	reached := map[string]bool{indexFileName: true}
	var hop1 []string
	for _, link := range extractLinks(rootIdx) {
		p := resolveLink(indexFileName, link)
		if !reached[p] {
			reached[p] = true
			hop1 = append(hop1, p)
		}
	}
	for _, p := range hop1 {
		content, ok := files[p]
		if !ok {
			continue
		}
		for _, link := range extractLinks(content) {
			reached[resolveLink(p, link)] = true
		}
	}

	var findings []LintFinding
	for p := range files {
		if p == indexFileName || strings.HasPrefix(p, episodesDir+"/") {
			continue
		}
		if !reached[p] {
			findings = append(findings, LintFinding{
				Path: p, Rule: "reachability",
				Detail: "not reachable within 2 hops of INDEX.md",
			})
		}
	}
	return findings
}

// lintNoPayloadInRouters flags a router file (INDEX.md / */INDEX.md)
// carrying a concept-payload section heading or a long bullet run instead of
// pure routing (headings, tables, links).
func lintNoPayloadInRouters(files map[string]string) []LintFinding {
	var findings []LintFinding
	for p, c := range files {
		if p != indexFileName && !strings.HasSuffix(p, "/"+indexFileName) {
			continue
		}
		if payloadHeadingRe.MatchString(c) {
			findings = append(findings, LintFinding{
				Path: p, Rule: "no-payload-in-router",
				Detail: "router file contains a concept-payload section heading",
			})
			continue
		}
		if n := longBulletRunLength(c); n > routerBulletBudget {
			findings = append(findings, LintFinding{
				Path: p, Rule: "no-payload-in-router",
				Detail: fmt.Sprintf("%d consecutive bullet lines — routers should be tables/links, not lists", n),
			})
		}
	}
	return findings
}

// longBulletRunLength returns the longest run of consecutive "- "/"* " lines.
func longBulletRunLength(content string) int {
	best, cur := 0, 0
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

// lintSizeBudget flags files exceeding their ICM layer's size budget.
func lintSizeBudget(files map[string]string) []LintFinding {
	var findings []LintFinding
	for p, c := range files {
		size := len(c)
		switch {
		case p == identityFile:
			if size > maxIdentityBytes {
				findings = append(findings, sizeFinding(p, size, maxIdentityBytes, "identity"))
			}
		case strings.HasPrefix(p, referenceDir+"/"), strings.HasPrefix(p, conventionsDir+"/"):
			if strings.HasSuffix(p, "/"+indexFileName) {
				continue
			}
			if size > maxConceptBytes {
				findings = append(findings, sizeFinding(p, size, maxConceptBytes, "concept"))
			}
		case strings.HasPrefix(p, episodesDir+"/"):
			if size > maxEpisodeBytes {
				findings = append(findings, sizeFinding(p, size, maxEpisodeBytes, "episode"))
			}
		}
	}
	return findings
}

func sizeFinding(path string, size, budget int, layer string) LintFinding {
	return LintFinding{
		Path: path, Rule: "size-budget",
		Detail: fmt.Sprintf("%d bytes exceeds %s budget of %d bytes", size, layer, budget),
	}
}

// lintEntryFiles flags a harness entry file's MOM-managed block carrying
// concept payload instead of pure routing.
func lintEntryFiles(root string) []LintFinding {
	var findings []LintFinding
	for _, name := range []string{"CLAUDE.md", "AGENTS.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		content := string(data)
		bIdx := strings.Index(content, blockBeginMarker)
		eIdx := strings.Index(content, blockEndMarker)
		if bIdx < 0 || eIdx <= bIdx {
			continue
		}
		block := content[bIdx+len(blockBeginMarker) : eIdx]
		if payloadHeadingRe.MatchString(block) {
			findings = append(findings, LintFinding{
				Path: name, Rule: "no-payload-in-router",
				Detail: "MOM-managed entry block contains a concept-payload section heading",
			})
		}
	}
	return findings
}
