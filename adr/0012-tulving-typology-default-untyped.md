# 0012 — Tulving typology for memory types; default to untyped

Memories vary in kind: a captured fact, a recurring pattern, a single-event recollection, a learned procedure. The current schema flattens this — every memory is just "a memory" — which forces recall to rely entirely on tags and FTS5 to distinguish "the time we decided X" from "the rule we always follow."

MOM adopts a typology adapted from Tulving's memory taxonomy. The `type` enum on `memories` is `episodic | semantic | procedural | untyped`: `semantic` for facts, definitions, and stable knowledge; `episodic` for specific events, decisions, and single-occurrence recollections; `procedural` for how-to, recurring patterns, and rules; `untyped` for memories that have not been classified yet. The set is intentionally small; finer distinctions belong in tags.

`type` is **operational metadata**, not substance (ADR 0011) — it is mutable and can be assigned post-hoc. **The default is `untyped`.** Defaulting to a real type would silently assert a classification that Drafter has no basis to make. `untyped` is honest: it says "no one has decided what this is yet," which makes it queryable ("show me everything that needs typing") and keeps the type field meaningful when it is set.

Type assignment happens in three places: (a) the user sets it explicitly when creating or curating; (b) wrap-up surfaces untyped drafts and suggests a type using deterministic heuristics, with the user picking; (c) external agents connected through MCP can suggest types programmatically, but the human (or wrap-up acting on their behalf) confirms. Drafter itself never assigns a type.

## Consequences

- Recall can filter by type cheaply ("procedural memories tagged `recall`") without overloading tags to encode kind.
- The vault honestly reflects which memories have been curated and which are still raw drafts. `untyped` becomes a curation backlog signal, not a bug.
- Wrap-up's role expands: it is now the canonical local curation surface, suggesting types alongside its existing promotion and landmark suggestions. The heuristics are deterministic; the choices are human.
- Type can be changed later without rewriting substance, which is exactly what ADR 0011 makes safe.
- A four-value enum is small enough that recall code paths stay legible; finer-grained taxonomies (e.g. distinguishing decisions from observations) are expressed as tags on top.

## Considered alternatives

- **Default to `semantic`, override available.** Rejected: silently classifies everything as fact-shaped, which is wrong for most captured turns and erodes the type field's signal.
- **Require explicit declaration at creation.** Rejected: capture does not have the context to declare a type, and forcing the user to type every captured turn at creation time defeats the point of automatic capture.
- **Deterministic heuristics at capture (regex/keyword rules).** Rejected: capture-time heuristics are wrong often enough that the type field becomes unreliable. Heuristics belong in wrap-up where they are advisory and human-confirmed.
- **A larger typology (e.g. decisions, observations, references, procedures, glossary, plans).** Rejected: the boundaries blur and disagreements between users compound. Four buckets plus tags covers the same surface with less argument.
