# 0026 — ICM-faithful vault structure and document ingestion

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

Separately, MOM had no path for durable knowledge that doesn't originate from
a captured coding session — books, docs, framework write-ups an agent should
reason from. MOM ships as a single binary; it cannot depend on a third-party
Python skill being installed on the user's machine, so it needs to parse
documents itself. (The `book-to-skill` skill's frameworks/mental-models/
principles/techniques/anti-patterns extraction shape was the design
reference for what MOM's own extractor produces.) MOM also needed a way to
fold that extraction into the vault without introducing a second,
non-regenerable knowledge tree that breaks the "vault is a pure projection of
the Ledger" invariant (ADR 0021, ADR 0024).

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

### Documents fold into one concept per book, never a sidecar tree

`mom ingest <file>` parses the file with MOM's own pure-Go extractor
(`ingress/docparse`) and appends one `capture.document_chapter.observed`
Ledger event per chapter — bounded in size, deduped by content-hash `doc_id`,
so re-ingesting the same file is a no-op. `docparse` natively handles `.txt`
`.md` `.html` `.epub` (spine-ordered) and `.docx`; `--text` treats any other
input as plain text/markdown after the user converts it themselves; anything
else is rejected with an actionable error. PDF is deliberately not a native
format: a weak PDF parser writes plausible-looking garbage into an
append-only Ledger, and bad memory cannot be retracted — the vault is
regenerated *from* those events, so a bad extraction poisons every future
fold rather than one. Requiring a manual `pdftotext` conversion keeps that
failure mode a visible, one-time human decision instead of a silent parser
guess baked permanently into the Ledger. This keeps ingestion inside the
existing write path: the Ledger is still the sole source of truth, and
`mom vault rebuild` still regenerates everything, including ingested books,
from offset 0. The fold synthesizes a book's chapter events into exactly one
`reference/<book-slug>.md` concept (`subtype: document`, layer B), organized
by MOM's own fold-prompt taxonomy (frameworks / mental models / principles /
techniques / anti-patterns — `services/projection/hierarchy.go`), and
cross-links it into the subject concepts it informs rather than merging the
two — a book stays a distinct, addressable concept. Ingested books surface
under a 📖 Documents section of `reference/INDEX.md`.

## Considered alternatives

- **Take a pure-Go PDF-parsing dependency now, instead of deferring PDF.**
  Rejected: every pure-Go PDF library available at time of writing trades
  correctness for coverage on real-world PDFs (scanned pages, complex
  layouts, non-standard encodings) and fails silently rather than loudly —
  it returns *some* text instead of an error. That is the wrong failure mode
  for an append-only, non-retractable Ledger: a loud "unsupported format,
  convert first" error costs the user one command; a quiet bad extraction
  costs every future fold that touches that book. Deferring PDF until a
  library clears that bar is asymmetric in the safe direction.

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
- **Documents are Ledger-native, not a bolt-on.** `mom ingest` adds an event
  type and a synthesis path; it does not add a second storage mechanism. The
  "vault is a fully-regenerable projection of the Ledger" invariant from
  ADR 0021/0024 holds for books exactly as it does for captured sessions.
