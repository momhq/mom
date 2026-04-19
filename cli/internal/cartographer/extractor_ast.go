package cartographer

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// extIndex is built once at init time for O(1) extension lookup.
var extIndex = buildExtensionIndex(astLanguageRegistry)

// TreeSitterASTExtractor extracts type/function declarations using tree-sitter.
// Supports Go, Python, JavaScript, TypeScript, TSX, Rust, Java, Ruby, C, C++, C#.
type TreeSitterASTExtractor struct{}

// NewTreeSitterASTExtractor returns an initialised TreeSitterASTExtractor.
func NewTreeSitterASTExtractor() *TreeSitterASTExtractor {
	return &TreeSitterASTExtractor{}
}

func (e *TreeSitterASTExtractor) Name() string { return "ast" }

func (e *TreeSitterASTExtractor) Matches(path string) bool {
	ext := strings.ToLower(fileExt(path))
	_, ok := extIndex[ext]
	return ok
}

func (e *TreeSitterASTExtractor) Extract(ctx context.Context, src Source) ([]Draft, error) {
	ext := strings.ToLower(src.Extension)
	handler, ok := extIndex[ext]
	if !ok {
		return nil, nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(handler.language)

	tree, err := parser.ParseCtx(ctx, nil, src.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse (%s): %w", handler.name, err)
	}
	defer tree.Close()

	root := tree.RootNode()
	srcHash := hashBytes(src.Content)

	// Go uses a hand-written walker that also extracts doc comments.
	if handler.name == "go" {
		return extractGoDeclarations(root, src, srcHash), nil
	}

	return extractViaQuery(handler, root, src, srcHash)
}

// extractViaQuery uses a tree-sitter Scheme query to extract named symbols.
func extractViaQuery(h *languageHandler, root *sitter.Node, src Source, srcHash string) ([]Draft, error) {
	q, err := sitter.NewQuery([]byte(h.query), h.language)
	if err != nil {
		return nil, fmt.Errorf("ast query (%s): %w", h.name, err)
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(q, root)

	var drafts []Draft
	seen := make(map[string]bool) // deduplicate (same name, same line)

	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		for _, cap := range m.Captures {
			node := cap.Node
			name := node.Content(src.Content)
			if name == "" {
				continue
			}

			line := int(node.StartPoint().Row) + 1
			endLine := int(node.EndPoint().Row) + 1

			dedupeKey := fmt.Sprintf("%s:%d", name, line)
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true

			kind := symbolKind(h.name, node)

			drafts = append(drafts, Draft{
				Type:       "fact",
				Summary:    fmt.Sprintf("%s %s: %s", h.name, kind, name),
				Tags:       []string{h.name, kind, "ast", "bootstrap"},
				Confidence: ConfidenceExtracted,
				Content: map[string]any{
					"symbol":   name,
					"kind":     kind,
					"language": h.name,
				},
				Provenance: ProvenanceMeta{
					SourceFile:   src.Path,
					SourceLines:  lineRange(line, endLine),
					SourceHash:   srcHash,
					TriggerEvent: TriggerEvent,
				},
			})
		}
	}

	return drafts, nil
}

// symbolKind maps a captured node's parent node type to a human-readable kind string.
func symbolKind(lang string, node *sitter.Node) string {
	parent := node.Parent()
	if parent == nil {
		return "symbol"
	}
	return nodeTypeToKind(lang, parent.Type())
}

// nodeTypeToKind converts a tree-sitter node type string to a canonical kind label.
func nodeTypeToKind(lang, nodeType string) string {
	switch nodeType {
	// Shared across many languages
	case "class_declaration", "class_definition", "class_specifier", "class":
		return "class"
	case "function_declaration", "function_definition", "function_item":
		return "function"
	case "method_declaration", "method_definition", "singleton_method":
		return "method"
	case "interface_declaration":
		return "interface"
	case "enum_declaration", "enum_item":
		return "enum"
	case "struct_specifier", "struct_item":
		return "struct"
	case "trait_item":
		return "trait"
	case "impl_item":
		return "impl"
	case "module":
		return "module"
	case "record_declaration":
		return "record"
	case "type_definition":
		return "typedef"
	case "lexical_declaration", "variable_declaration":
		return "const"
	default:
		return nodeType
	}
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
