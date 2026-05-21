---
name: mom-wrap-up
description: Curate recent MOM drafts. Use when user asks to wrap up, finish, close the session, preserve decisions, or prepare memory before clearing context.
user-invocable: true
allowed-tools: Bash(mom drafts*), Bash(mom curate*), Bash(command -v mom*), Bash(brew install momhq/tap/mom*)
---

Invoking this skill **is** the user's request to wrap up. Proceed with the flow below immediately — do not ask the user to confirm they want to wrap up.

## Preflight

Check that `mom` is on PATH:

```bash
command -v mom
```

If it is missing, tell the user MOM is not installed and ask permission to install it:

```text
MOM is not installed. Install it now with Homebrew?
  brew install momhq/tap/mom
Source: https://github.com/momhq/mom
```

If the user agrees, run that command. If the user declines, stop. Do not install MOM without explicit permission.

## Flow

1. List recent drafts scoped to the current project. `--strict-project` is **required** on every invocation, no exceptions:

   ```bash
   mom drafts --strict-project
   ```

   If the user gives a time window (Go duration like `1h`, `30m`, `2d`), pass it as well:

   ```bash
   mom drafts --strict-project --since 1h
   ```

   **If the command returns zero drafts**, do not stop yet — perform a capture-pipeline sanity check. Re-run the same query without `--strict-project` (keep `--harness` and `--since` if you used them):

   ```bash
   mom drafts [--harness <name>] [--since <duration>]
   ```

   - If the unscoped query **also returns zero**: report "no drafts in this project for the requested window" and stop.
   - If the unscoped query **returns results**: report the following and stop — do **not** curate:

     ```
     ⚠ Capture-pipeline misconfiguration detected.
     Drafts exist for this session but carry no project_id — they cannot be
     project-scoped and will not be curated. This typically means the MOM
     watcher was not project-bound when these turns were captured.
     Run /mom-project to bind the project first and try re-run /mom-wrap-up.
     ```

   Do **not** widen the search to other projects on your own.

   Only add `--all-projects` if the user explicitly asks for a cross-project wrap-up. In that case, replace `--strict-project` with `--all-projects` (never run without one of the two).

   Other optional narrowing flags, only when context calls for it:
   - `--harness <name>` — restrict to one harness (`claude-code`, `codex`, `pi`)
   - `--session <id>` — restrict to one session when the user knows the id

   The output has columns `ID  Created  Harness  Project  Summary`.

2. Propose a curation plan. For each draft worth keeping, list:
   - draft id
   - type: `semantic`, `procedural`, or `episodic`
   - approved summary
   - one-line reason to keep it

   Hide drafts you recommend discarding unless the user asks to see them.

3. Wait for the user to approve. Do not curate anything before approval.

4. For each approved draft, run:

   ```bash
   mom curate <id> --type <semantic|procedural|episodic> --summary "<approved summary>"
   ```

5. Report when done:

   ```text
   ## Wrap-up complete
   Curated:  <N>
   Deferred: <N or none>
   ```

## Rules

- Always include `--strict-project` on every `mom drafts` call. Zero results **does not** mean "try a wider search" — it means "run the sanity check" (see Flow step 1).
- If `mom drafts --strict-project` fails with `unknown flag: --strict-project`, the installed MOM is too old. Tell the user to upgrade with `brew upgrade mom` (or `mom self-update`) and stop. **Do not** fall back to a bare `mom drafts` — that would leak drafts from other projects.
- **CLI flag surface — never invent flags.** Before using any flag on any `mom` subcommand, run `<subcommand> --help` and confirm the flag appears in the output. If it is not listed, do not use it. Never assume a flag exists based on convention or analogy with other CLIs.
- Never curate without user approval.
- Never skip `--type` or `--summary`.
- Do not rewrite draft content.
- Do not use MCP or run ad hoc database queries — only the commands above.

## Rehearsal scenarios

Use these to verify the skill behaves correctly without a live session.

**Scenario A — genuinely empty session**
Both `mom drafts --strict-project` and `mom drafts` return zero rows.
Expected skill output: `"no drafts in this project for the requested window"` → stop.

**Scenario B — capture-pipeline misconfiguration**
`mom drafts --strict-project` returns zero rows. `mom drafts` (without `--strict-project`) returns rows whose `Project` column is blank/empty.
Expected skill output: `"⚠ Capture-pipeline misconfiguration detected …"` → stop, do not curate.

**Scenario C — normal session**
`mom drafts --strict-project` returns rows with populated `Project` column.
Expected skill output: curation plan proposal → wait for user approval → curate.

## Postflight (version hint)

Any `mom ...` command may print a banner to stderr like:

```
MOM 0.40.1 available. Run `brew upgrade mom` or `mom self-update`
```

If you see that line, finish the task first, then add one short line at the end of your reply suggesting the upgrade. Do not run the upgrade yourself.
