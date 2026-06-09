---
name: mom-fold
description: Fold this project's newly captured sessions into the MOM markdown vault. Use when the user wants to wrap up, finish, close the session, save decisions, update memory, or prepare the vault before clearing context.
user-invocable: true
allowed-tools: Bash(mom vault fold*), Bash(mom vault status*), Bash(test -f .mom-project.yaml*), Bash(command -v mom*), Bash(brew install momhq/tap/mom*)
---

Invoking this skill **is** the user's request to wrap up. Proceed with the flow below immediately — do not ask the user to confirm they want to fold.

Folding reads the events captured since the last fold and synthesizes them into the project's markdown vault under `.mom/vault/`. It is the end-of-session step that turns raw captured turns into navigable memory.

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

Confirm a project is bound — capture and fold only work for a bound directory:

```bash
test -f .mom-project.yaml
```

If it is missing, the directory is unbound and nothing has been captured. Tell the user to bind first with `/mom-project`, then stop.

## Run

```bash
mom vault fold
```

Folding is incremental — it only processes events newer than the last watermark — and uses an LLM engine, so it can take a moment. When it finishes, run:

```bash
mom vault status
```

and report a one-line summary: the new watermark offset, files written, and last fold time.

## Rules

- Do not pass an `--engine` flag unless the user explicitly asked for a specific engine.
- If `mom vault fold` reports there is nothing new, say so — folding again is a safe no-op.
- Never edit files under `.mom/vault/` by hand from this skill; folding owns them.
- **CLI flag surface — never invent flags.** Before using any flag on any `mom` subcommand, run `<subcommand> --help` and confirm the flag appears. If it is not listed, do not use it.

## Postflight (version hint)

Any `mom ...` command may print a banner to stderr like:

```
MOM 0.40.1 available. Run `brew upgrade mom` or `mom self-update`
```

If you see that line, finish the task first, then add one short line at the end of your reply suggesting the upgrade. Do not run the upgrade yourself.
