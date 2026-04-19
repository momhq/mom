package cartographer

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// astExtensions maps file extension to language name.
var astExtensions = map[string]string{
	".go": "go",
}

// TreeSitterASTExtractor extracts type/function declarations using tree-sitter.
// v0.8.0 ships Go support only; other languages are follow-up issues.
type TreeSitterASTExtractor struct {
	parsers map[string]*sitter.Language
}

// NewTreeSitterASTExtractor returns an initialised TreeSitterASTExtractor.
func NewTreeSitterASTExtractor() *TreeSitterASTExtractor {
	return &TreeSitterASTExtractor{
		parsers: map[string]*sitter.Language{
			"go": golang.GetLanguage(),
		},
	}
}

func (e *TreeSitterASTExtractor) Name() string { return "ast" }

func (e *TreeSitterASTExtractor) Matches(path string) bool {
	ext := strings.ToLower(fileExt(path))
	_, ok := astExtensions[ext]
	return ok
}

func (e *TreeSitterASTExtractor) Extract(ctx context.Context, src Source) ([]Draft, error) {
	ext := strings.ToLower(src.Extension)
	langName, ok := astExtensions[ext]
	if !ok {
		return nil, nil
	}

	lang, ok := e.parsers[langName]
	if !ok {
		return nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(ctx, nil, src.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	srcHash := hashBytes(src.Content)

	switch langName {
	case "go":
		return extractGoDeclarations(root, src, srcHash), nil
	}
	return nil, nil
}

// extractGoDeclarations walks a Go AST and extracts top-level declarations.
func extractGoDeclarations(root *sitter.Node, src Source, srcHash string) []Draft {
	var drafts []Draft
	lines := linesOf(src.Content)

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "type_declaration":
			drafts = append(drafts, goTypeDraft(child, src, srcHash, lines)...)
		case "function_declaration":
			d := goFuncDraft(child, src, srcHash, lines)
			if d != nil {
				drafts = append(drafts, *d)
			}
		case "method_declaration":
			d := goMethodDraft(child, src, srcHash, lines)
			if d != nil {
				drafts = append(drafts, *d)
			}
		}
	}
	return drafts
}

// goTypeDraft produces a fact draft for a top-level Go type declaration.
func goTypeDraft(node *sitter.Node, src Source, srcHash string, lines []string) []Draft {
	startLine := int(node.StartPoint().Row)
	endLine := int(node.EndPoint().Row)

	// Find the type name.
	name := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		if c.Type() == "type_spec" {
			for j := 0; j < int(c.ChildCount()); j++ {
				cc := c.Child(j)
				if cc != nil && cc.Type() == "type_identifier" {
					name = cc.Content(src.Content)
					break
				}
			}
		}
	}

	if name == "" {
		return nil
	}

	// Extract leading comment (doc comment) if present.
	docComment := extractLeadingComment(node, lines)

	summary := fmt.Sprintf("Type: %s", name)
	if docComment != "" {
		summary = fmt.Sprintf("Type: %s — %s", name, truncate(docComment, 100))
	}

	content := map[string]any{
		"name":     name,
		"language": "go",
		"kind":     "type",
	}
	if docComment != "" {
		content["doc"] = docComment
	}

	return []Draft{{
		Type:       "fact",
		Summary:    summary,
		Tags:       []string{"type", "go", "ast", "bootstrap"},
		Confidence: ConfidenceExtracted,
		Content:    content,
		Provenance: ProvenanceMeta{
			SourceFile:   src.Path,
			SourceLines:  lineRange(startLine+1, endLine+1),
			SourceHash:   srcHash,
			TriggerEvent: TriggerEvent,
		},
	}}
}

// goFuncDraft produces a fact draft for a top-level Go function declaration.
func goFuncDraft(node *sitter.Node, src Source, srcHash string, lines []string) *Draft {
	startLine := int(node.StartPoint().Row)
	endLine := int(node.EndPoint().Row)

	name := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c != nil && c.Type() == "identifier" {
			name = c.Content(src.Content)
			break
		}
	}
	if name == "" {
		return nil
	}

	// Skip unexported functions.
	if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
		return nil
	}

	docComment := extractLeadingComment(node, lines)
	summary := fmt.Sprintf("Function: %s", name)
	if docComment != "" {
		summary = fmt.Sprintf("Function: %s — %s", name, truncate(docComment, 100))
	}

	content := map[string]any{
		"name":     name,
		"language": "go",
		"kind":     "function",
	}
	if docComment != "" {
		content["doc"] = docComment
	}

	return &Draft{
		Type:       "fact",
		Summary:    summary,
		Tags:       []string{"function", "go", "ast", "bootstrap"},
		Confidence: ConfidenceExtracted,
		Content:    content,
		Provenance: ProvenanceMeta{
			SourceFile:   src.Path,
			SourceLines:  lineRange(startLine+1, endLine+1),
			SourceHash:   srcHash,
			TriggerEvent: TriggerEvent,
		},
	}
}

// goMethodDraft produces a fact draft for a top-level Go method declaration.
func goMethodDraft(node *sitter.Node, src Source, srcHash string, lines []string) *Draft {
	startLine := int(node.StartPoint().Row)
	endLine := int(node.EndPoint().Row)

	name := ""
	receiver := ""

	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c == nil {
			continue
		}
		if c.Type() == "field_identifier" {
			name = c.Content(src.Content)
		}
		if c.Type() == "parameter_list" && receiver == "" {
			// Receiver is the first parameter list.
			receiver = c.Content(src.Content)
		}
	}

	if name == "" {
		return nil
	}

	// Skip unexported methods.
	if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
		return nil
	}

	docComment := extractLeadingComment(node, lines)
	summary := fmt.Sprintf("Method: %s%s", receiver, name)
	if docComment != "" {
		summary = fmt.Sprintf("Method: %s%s — %s", receiver, name, truncate(docComment, 100))
	}

	content := map[string]any{
		"name":     name,
		"receiver": receiver,
		"language": "go",
		"kind":     "method",
	}
	if docComment != "" {
		content["doc"] = docComment
	}

	return &Draft{
		Type:       "fact",
		Summary:    summary,
		Tags:       []string{"method", "go", "ast", "bootstrap"},
		Confidence: ConfidenceExtracted,
		Content:    content,
		Provenance: ProvenanceMeta{
			SourceFile:   src.Path,
			SourceLines:  lineRange(startLine+1, endLine+1),
			SourceHash:   srcHash,
			TriggerEvent: TriggerEvent,
		},
	}
}

// extractLeadingComment returns the text of a Go doc comment immediately
// preceding node, if any. It looks at the lines immediately above node's start.
func extractLeadingComment(node *sitter.Node, lines []string) string {
	startLine := int(node.StartPoint().Row)
	if startLine == 0 {
		return ""
	}

	var commentLines []string
	for i := startLine - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "//") {
			commentLines = append([]string{strings.TrimPrefix(strings.TrimPrefix(line, "//"), " ")}, commentLines...)
		} else {
			break
		}
	}

	return strings.TrimSpace(strings.Join(commentLines, " "))
}
