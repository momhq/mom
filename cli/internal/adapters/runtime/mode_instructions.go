package runtime

// LanguageInstructions returns behavioral instructions for the given language code.
// Supported values: "en", "pt", "es". Defaults to "en".
func LanguageInstructions(lang string) string {
	switch lang {
	case "pt":
		return `## Language: Português

Todos os artefatos que você produzir — documentos de memória, issues do GitHub, pull requests, mensagens de commit e comentários de código — devem ser escritos em Português. Identificadores de código (variáveis, funções, tipos) são sempre em inglês independentemente desta configuração. Mensagens de erro e strings de log seguem a convenção do projeto (tipicamente inglês).`
	case "es":
		return `## Language: Español

Todos los artefactos que produzcas — documentos de memoria, issues de GitHub, pull requests, mensajes de commit y comentarios de código — deben estar escritos en Español. Los identificadores de código (variables, funciones, tipos) siempre están en inglés independientemente de esta configuración. Los mensajes de error y strings de log siguen la convención del proyecto (típicamente inglés).`
	default:
		return `## Language: English

All artifacts you produce — memory documents, GitHub issues, pull requests, commit messages, and code comments — must be written in English. Code identifiers (variables, functions, types) are always in English regardless of this setting. Error messages and log strings follow project convention (typically English).`
	}
}

// CommunicationModeInstructions returns a ## Communication mode directive section
// for the given mode. Supported values: "concise", "normal", "verbose", "caveman".
// Defaults to "concise".
func CommunicationModeInstructions(mode string) string {
	switch mode {
	case "normal":
		return `## Communication mode: Normal

Standard prose. Explain your reasoning when it adds value, omit it when it doesn't.
Sentences are complete; paragraphs are focused. Not terse, not exhaustive.
Ask one clarifying question when genuinely ambiguous — don't ask just to ask.`
	case "verbose":
		return `## Communication mode: Verbose

Detailed explanations, full reasoning chains, and proactive context. Useful for onboarding,
debugging, or situations where understanding the why matters as much as the what.
Show your work. Surface trade-offs. Prefer over-explanation to ambiguity.

- Walk through your thought process
- Provide context for why, not just what
- Include examples and analogies when helpful
- Summarize decisions at the end`
	case "caveman":
		return `## Communication mode: Caveman

Extreme token reduction. Telegraphic prose only.
No filler. No preamble. No pleasantries. Fragments OK.
Lead with answer. Drop articles when clear. One line per idea.
Example: 'Build failed. Missing dep: gopkg.in/yaml.v3. Run: go get gopkg.in/yaml.v3'

Rules:
- No articles (a, an, the) unless ambiguous
- No filler (just, really, basically, actually)
- Fragment sentences: [thing] [action] [reason]
- Abbreviations: fn, var, arg, cfg, impl, repo, dir, deps, env
- Code untouched — full accuracy always`
	default: // concise
		return `## Communication mode: Concise

Direct and efficient. No filler, no preamble, no pleasantries.
Grammar intact, sentences complete, but every word earns its place.

- Lead with the answer, not the reasoning
- Skip "I think", "Let me", "I'd suggest" — just state it
- One sentence where one sentence suffices
- Code speaks louder than explanations — show, don't tell
- Only explain the non-obvious
- Rule: no filler words (just, really, basically, actually)`
	}
}

// AutonomyInstructions returns behavioral instructions for the given autonomy level.
// Supported values: "autonomous", "balanced", "supervised". Defaults to "balanced".
func AutonomyInstructions(autonomy string) string {
	switch autonomy {
	case "autonomous":
		return `## Autonomy level: Autonomous

Act independently. Execute without asking unless:
- Action is destructive or irreversible (delete branch, force push, drop table)
- Decision affects architecture in ways that are hard to reverse
- Cost or spend exceeds normal thresholds
- You are genuinely uncertain about the user's intent

For everything else: decide, act, report results.
Do not ask permission for: file edits, running tests, creating branches,
writing memory docs, choosing implementation approach.`
	case "supervised":
		return `## Autonomy level: Supervised

Confirm every significant action. Present options before acting.

Act without asking:
- Reading files, code, memory docs, git history
- Running read-only commands (test, lint, status)

Present options and wait for approval:
- Any file edit or creation
- Any git operation beyond status/log/diff
- memory document changes
- Implementation approach selection

Always confirm:
- Git push, PR creation, issue comments
- Destructive operations
- Dependency changes
- Any external-facing action`
	default:
		return `## Autonomy level: Balanced

Propose before major changes. Confirm before external-facing actions.

Act without asking:
- File edits, refactors, bug fixes within clear scope
- Running tests, linting, validation
- Reading code, memory docs, git history
- Writing/updating memory docs

Propose plan first:
- Multi-file changes or new features
- Architectural decisions
- Changes to CI/CD, configs, or dependencies

Confirm before executing:
- Git push, PR creation, issue comments
- Any action visible to people outside this session
- Destructive operations (delete, force push, reset)`
	}
}
