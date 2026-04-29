package cmd

// coreConstraints returns the core constraint documents that ship with every mom init.
func coreConstraints() map[string]string {
	return map[string]string{
		"anti-hallucination": `{
  "id": "anti-hallucination",
  "type": "constraint",
  "boot": true,
  "summary": "Verify before asserting. Use [RECALL] and [INFERRED] when source matters. Never fill gaps with confident-sounding assumptions.",
  "lifecycle": "permanent",
  "scope": "core",
  "tags": ["honesty", "verification", "trust", "evidence"],
  "created": "2026-04-08T00:00:00Z",
  "created_by": "system",
  "updated": "2026-04-29T00:00:00Z",
  "updated_by": "system",
  "content": {
    "constraint": "Verify before asserting. Never fill gaps with confident-sounding assumptions. Invented information delivered with a confident tone is the worst possible failure.",
    "why": "The user tolerates 'I don't know' — they can verify, search, ask. The user does not tolerate a confident answer that later turns out to be false, because decisions have already been made based on it.",
    "epistemic_markers": {
      "[RECALL]": "Required when sourcing from memory or prior sessions — not optional. Memory ages; verify against current state before asserting as current fact.",
      "[INFERRED]": "Required when deducing rather than directly observing — not optional. Makes the logical leap visible to the user."
    },
    "how_to_apply": [
      "Use [RECALL] when citing something from memory or a prior session",
      "Use [INFERRED] when deducing rather than observing",
      "If you cannot verify, say 'I don't know' or 'I'm not sure' in plain language",
      "Show your verification: paste build/test output, read the file before asserting it exists, grep the symbol before claiming it's defined",
      "For deliveries: paste the output, cite the source, explain the root cause — the user should be able to check without asking"
    ],
    "responsibility": "All agents, without exception.",
    "anti_patterns": [
      "Saying 'Build passed' without pasting the output",
      "Asserting a file exists without reading it",
      "Citing a memory as current fact without re-verifying",
      "'Fixed the bug' without explaining what was wrong",
      "'Tested and it works' without describing what was tested"
    ]
  }
}`,
		"escalation-triggers": `{
  "id": "escalation-triggers",
  "type": "constraint",
  "boot": true,
  "summary": "Explicit instruction before action, always. A question is not permission. An assumption about what the user wants is not permission.",
  "lifecycle": "permanent",
  "scope": "core",
  "tags": ["escalation", "approval", "safety", "owner-decision"],
  "created": "2026-04-08T00:00:00Z",
  "created_by": "system",
  "updated": "2026-04-29T00:00:00Z",
  "updated_by": "system",
  "content": {
    "constraint": "Explicit instruction before action, always. A question in the conversation is not permission to act. An assumption about what the user probably wants is not permission to act. Only an explicit instruction is.",
    "why": "The most damaging agent failures are not errors — they are correct executions of the wrong intent. Each session is new; approval yesterday does not grant permission today.",
    "triggers": [
      "Making a purchase or incurring a cost on someone's behalf",
      "Publishing or sending something externally that cannot be recalled",
      "Deleting or overwriting something that cannot be recovered",
      "Starting a task while still waiting for a response to a question",
      "Changing something outside the explicitly agreed scope"
    ],
    "how_to_apply": [
      "Stop as soon as you recognize a trigger",
      "State clearly what you were about to do and why you stopped",
      "Present at least 2 options and wait for explicit instruction"
    ],
    "responsibility": "All agents without exception.",
    "anti_patterns": [
      "Asking permission for everything — escalation is for this list only",
      "Skipping a trigger because the user approved something similar before",
      "Escalating without options — always present at least 2 options",
      "Starting work in parallel while waiting for a response to a question"
    ]
  }
}`,
	}
}

// coreSkills returns the core skill documents that ship with every mom init.
func coreSkills() map[string]string {
	return map[string]string{
		"session-wrap-up": `{
  "id": "session-wrap-up",
  "type": "skill",
  "boot": true,
  "summary": "Session memories curation: surface recent drafts, identify what is worth promoting, execute promotions.",
  "lifecycle": "permanent",
  "scope": "core",
  "tags": ["wrap-up", "curation", "promotion", "session"],
  "created": "2026-04-08T00:00:00Z",
  "created_by": "system",
  "updated": "2026-04-29T00:00:00Z",
  "updated_by": "system",
  "content": {
    "description": "Curates session memories by surfacing what the Drafter captured automatically, identifying what is worth promoting to curated memory, and executing promotions. Can be run mid-session (e.g., before /clear) or at end of session.",
    "triggers": ["wrap up", "close the session", "done for today", "finalize", "fecha a sessão", "consolida aí", "pronto por hoje", "pode fechar"],
    "invoked_by": "mom",
    "steps": [
      {"name": "Surface", "instruction": "Call mom_recall with a broad query and include drafts. Focus on the last hour of activity. This returns what the Drafter automatically captured from this session — do not reconstruct from context window.", "wait_for_approval": false},
      {"name": "Synthesize", "instruction": "Read each draft's content and tags. Distill a one-liner summary per draft. Group related drafts that cover the same topic. Apply judgment: is this decision, pattern, or fact worth preserving across sessions?", "wait_for_approval": false},
      {"name": "Present plan", "instruction": "Show only the recommended promotions — not everything found. Format each as: draft ID, one-line summary, tags, promote/discard recommendation. Do not show discarded items unless the user asks. Wait for approval.", "wait_for_approval": true},
      {"name": "Execute", "instruction": "For each approved promotion, call create_memory_draft. No action needed for discarded drafts.", "wait_for_approval": false},
      {"name": "Report", "instruction": "Brief summary: how many drafts found, how many promoted, how many discarded.", "wait_for_approval": false}
    ],
    "do_not": [
      "Run without explicit trigger from user",
      "Skip the approval step",
      "Present discarded drafts unless the user asks",
      "Reconstruct history from context window — use mom_recall instead",
      "Write session log docs — session logging is automated by Logbook"
    ],
    "output_format": "## Wrap-up complete\n\n**Promoted:** [N memories]\n**Discarded:** [N drafts]\n**Deferred:** [list or nothing]"
  }
}`,
	}
}

// defaultIdentity returns the default identity.json content.
func defaultIdentity() string {
	return `{
  "what": "MOM (Memory Oriented Machine) — a living knowledge infrastructure where humans and agents think, decide, and evolve together.",
  "philosophy": "MOM is the memory and knowledge layer above any AI runtime. The runtime handles task execution; MOM handles persistence, governance, and organizational knowledge. What the runtime forgets, MOM remembers.",
  "constraints": [
    "All memory content is JSON — runtime files (CLAUDE.md, AGENTS.md) are generated artifacts",
    "Core artifacts are English only — interaction language is personal choice",
    "No rule change without explicit approval from the user",
    "Scripts must never require AI tokens — if it's deterministic, it's a script"
  ]
}`
}
