package watcher

import "strings"

// CategorizeToolCall buckets a tool name into one of the stable
// categories the lens shows. The category is pre-computed by the watcher
// (the only component that sees individual tool names) and rides on the
// turn.observed event, so it is the boundary between harness-specific tool
// vocabulary and the dashboard's stable category model.
//
// Five buckets, by convention used in lens panels:
//
//	mom_memory     — historical MOM MCP memory tools (pre-v0.50 transcripts)
//	mom_cli        — mom-specific CLI invocations
//	codebase_read  — reads of repo content
//	codebase_write — writes to repo content
//	system         — everything else (Bash, Glob, harness internals…)
func CategorizeToolCall(toolName string) string {
	name := NormalizeToolName(toolName)
	switch {
	case isMemoryTool(name):
		return "mom_memory"
	case isMomCLI(name):
		return "mom_cli"
	case isCodebaseRead(name):
		return "codebase_read"
	case isCodebaseWrite(name):
		return "codebase_write"
	default:
		return "system"
	}
}

// NormalizeToolName strips harness-specific prefixes from tool names.
// Claude Code namespaces MCP tools as "mcp__<server>__<tool>"; this
// returns the bare tool name so categorisation is harness-agnostic.
func NormalizeToolName(toolName string) string {
	if strings.HasPrefix(toolName, "mcp__") {
		if i := strings.Index(toolName[5:], "__"); i >= 0 {
			return toolName[5+i+2:]
		}
	}
	return toolName
}

// isMemoryTool recognises MOM's retired MCP memory tools. MOM no longer
// ships an MCP server (v0.50), so these names only appear in historical
// transcripts captured before the cutover; categorising them keeps the
// lens breakdown correct for older sessions.
func isMemoryTool(name string) bool {
	return name == "mom_recall" ||
		name == "mom_record" ||
		name == "mom_get" ||
		name == "mom_landmarks" ||
		name == "mom_status"
}

// CategorizeObservedToolCall returns the Lens category and privacy-safe name
// for one observed tool call. It may inspect raw shell command input while the
// event is still on the in-process bus, but it returns only coarse metadata.
func CategorizeObservedToolCall(toolName string, input map[string]any) (category, safeName string) {
	name := NormalizeToolName(toolName)
	if isShellTool(name) {
		if momName := safeMomCLIName(input); momName != "" {
			return "mom_cli", momName
		}
	}
	return CategorizeToolCall(name), name
}

func isMomCLI(name string) bool {
	return name == "mom_draft" || name == "mom_log"
}

func isShellTool(name string) bool {
	return name == "Bash" || name == "bash" || name == "Shell" || name == "shell"
}

func safeMomCLIName(input map[string]any) string {
	if input == nil {
		return ""
	}
	command, _ := input["command"].(string)
	fields := strings.Fields(strings.TrimSpace(command))
	atCommandStart := true
	for i := 0; i < len(fields); i++ {
		field := strings.Trim(fields[i], "'")
		if isCommandSeparator(field) {
			atCommandStart = true
			continue
		}
		if !atCommandStart {
			continue
		}
		if field == "env" {
			continue
		}
		if isEnvAssignment(field) {
			continue
		}
		if field != "mom" {
			atCommandStart = false
			continue
		}
		if i+1 >= len(fields) {
			return "mom"
		}
		sub := strings.Trim(fields[i+1], "'")
		if sub == "" || strings.HasPrefix(sub, "-") || isCommandSeparator(sub) {
			return "mom"
		}
		return "mom " + safeSubcommandName(sub)
	}
	return ""
}

func isCommandSeparator(field string) bool {
	return field == "&&" || field == ";" || field == "||"
}

func isEnvAssignment(field string) bool {
	if strings.HasPrefix(field, "-") {
		return false
	}
	idx := strings.Index(field, "=")
	if idx <= 0 {
		return false
	}
	for _, r := range field[:idx] {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func safeSubcommandName(sub string) string {
	var b strings.Builder
	for _, r := range sub {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func isCodebaseRead(name string) bool {
	return name == "Read" || name == "read" || name == "Grep" || name == "grep" ||
		name == "Glob" || name == "glob" || name == "rg"
}

func isCodebaseWrite(name string) bool {
	return name == "Edit" || name == "edit" || name == "Write" || name == "write"
}
