# 0026 — ICM-faithful vault structure

## Context

ADR 0024 adopted a markdown vault "in the spirit of" ICM (Interpretable
Context Methodology): concept files with `type`/`name`/`description`
frontmatter, rolled up by folder. ADR 0025 (v0.52.0-alpha, "The ICM vault")
tightened this into layered synthesis (L0 episodes → L1 `reference/`/
`contracts/` → L2 `identity.md`) with per-folder `INDEX.md` routers, but the
result was still an approximation: `INDEX.md` files carried inline concept
enumeration alongside their routing role, the layer was a bare numeric
`level:`, and `contracts/` used a name the ICM method reserves for a
different concept.

The method's author specifies ICM's canonical shape precisely: routing files
route and carry no payload; generated indexes enumerate; concepts declare a
canonical layer (A/B/C) with an access tier gating raw vs. distilled content;
and "contract" names step-governance, not process convention. Landing the
method loosely first and correcting it now — rather than getting it right in
ADR 0024/0025 — costs a second migration, but the alternative (drifting
further from the spec every release) costs more.

Document/book ingestion as a source of vault knowledge is a related but
separate concern, deferred to a later ADR — this one scopes only the vault's
internal shape.

## Decision

### Routing files hold no payload; generated indexes hold the catalog

`INDEX.md` (root and per-folder) is now routing-only: a table of "if you're
about to do X, go to Y" with no inline concept list. The exhaustive catalog
of concepts lives in the generated folder indexes (`reference/INDEX.md`,
`conventions/INDEX.md`), which are rebuilt from frontmatter on every fold.
`mom vault lint` enforces this split as a walk-test: every concept must be
reachable from a routing file in at most two reads, routing files must carry
no payload, and concepts must stay within size budgets.

### Layer A/B/C replaces numeric level, plus an access-tier privacy gate

Frontmatter `level:` (numeric) is replaced by `layer:`:

- **A** — `identity.md`, always loaded.
- **B** — `reference/` and `conventions/` concepts, loaded by task.
- **C** — `episodes/`, raw provenance, evidence-loaded last.

`access_tier:` (`distilled` for A/B, `raw` for C) is a privacy gate: raw
episode text is machine-only, never surfaced as distilled fact. `subtype:`
(e.g. `document`) further classifies a concept within its layer. Both
`layer` and `access_tier` are force-stamped by MOM from the file's path — not
trusted from the LLM's own output — in both the synthesis pass and the
re-render pass, so a model mislabel self-corrects on the next fold instead of
persisting.

### `conventions/` replaces `contracts/`

ICM reserves "contract" for step-governance (what a specific action commits
to). MOM's `contracts/` folder held process/workflow rules, which ICM calls
conventions. The folder is renamed; `type: contract` becomes
`type: convention`; the old `contracts/` directory is pruned on rebuild.

### MOM-owned routers in every enabled entry file; one human-owned file survives

A byte-identical "Route by what you're about to do" table is written into
both `CLAUDE.md` and `AGENTS.md` (whichever the enabled harnesses read)
inside `<!-- MOM:BEGIN -->` / `<!-- MOM:END -->` markers, overwritten on
every fold — no harness reads a router the others don't. Everything else in
those files, and the entirety of the new repo-root `CONTEXT.md`, is
human-owned: MOM seeds `CONTEXT.md` once and never overwrites it again. It is
the one hand-authored file that survives regeneration, for the project facts
no capture pipeline can derive.

## What this supersedes

| ADR | Subject | Status |
|---|---|---|
| 0024 | v0.50 markdown-vault memory | **Amended** — the vault's directory layout and frontmatter schema described there (`contracts/`, numeric `level:`, inline `INDEX.md` enumeration) are superseded by this ADR's ICM-conformant shape. The write path and Ledger-as-backbone invariant it establishes are unchanged. |

No prior ADR is fully superseded; this ADR corrects the vault's internal
shape without changing the write path (ADR 0020/0021), the ingress surface
(ADR 0025), or the top-level layout (ADR 0017).

## Consequences

- **Breaking for existing vaults.** The folder rename, frontmatter schema
  change, and rewritten entry-file managed blocks are not migrated in place.
  `mom vault rebuild` regenerates a conformant vault from the untouched
  Ledger — nothing captured is lost. This is alpha; there is no opt-in flag.
- **Self-correcting layer/access_tier.** Because MOM stamps these from path
  rather than trusting LLM output, a bad model call degrades to a wrong
  label that heals on the next fold, not a permanent misfile.
- **`mom vault lint` is a standing regression gate.** Any future change that
  reintroduces payload in a routing file, or breaks the two-read reachability
  guarantee, fails lint before it fails a human skimming the vault.
- **Document/book ingestion is deferred.** A path for durable knowledge that
  doesn't originate from a captured coding session — books, docs, framework
  write-ups — is out of scope here and will get its own ADR when it lands.
