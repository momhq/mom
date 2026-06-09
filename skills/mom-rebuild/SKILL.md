---
name: mom-rebuild
description: Rebuild this project's MOM vault from scratch over the full captured history. Use after switching synthesis engines, recovering from a corrupted vault, or when the user explicitly asks to rebuild or regenerate memory from the beginning.
user-invocable: true
allowed-tools: Bash(mom vault rebuild*), Bash(mom vault status*), Bash(test -f .mom-project.yaml*), Bash(command -v mom*), Bash(brew install momhq/tap/mom*)
argument-hint: "[--engine auto|claude|codex|pi]"
---

Rebuild discards the synthesized vault and re-creates it from offset 0 over the entire captured history. It is heavier than a fold — use it deliberately. For routine end-of-session saves, use `/mom-fold` instead.

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

Confirm a project is bound:

```bash
test -f .mom-project.yaml
```

If it is missing, the directory is unbound — there is nothing to rebuild. Tell the user to bind first with `/mom-project`, then stop.

## Run

Confirm with the user before rebuilding — it regenerates the whole vault from scratch (human-authored content in CLAUDE.md is preserved; synthesized vault files are replaced) and runs the LLM engine over all history, so it can be slow.

On confirmation:

```bash
mom vault rebuild
```

Append `--engine <auto|claude|codex|pi>` only if the user named a specific engine. When it finishes, run `mom vault status` and report the watermark, file count, and engine used.

## Rules

- Always confirm before rebuilding — it is a full regeneration over all captured history.
- For routine saves use `/mom-fold`; reserve rebuild for engine changes or recovery from a corrupted vault.
- **CLI flag surface — never invent flags.** Before using any flag, run `mom vault rebuild --help` and confirm it appears.

## Postflight (version hint)

Any `mom ...` command may print a banner to stderr like:

```
MOM 0.40.1 available. Run `brew upgrade mom` or `mom self-update`
```

If you see that line, finish the task first, then add one short line at the end of your reply suggesting the upgrade. Do not run the upgrade yourself.
