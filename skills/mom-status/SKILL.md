---
name: mom-status
description: Show MOM's current state. Use when user asks if MOM is working, what MOM knows, to check setup, after context reset, or when MOM status is requested.
user-invocable: true
allowed-tools: Bash(mom status*), Bash(command -v mom*), Bash(brew install momhq/tap/mom*)
---

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

## Run

```bash
mom status
```

Report a short summary of: the bound project for the current directory (or that it is unbound), Ledger stats (total events and time span), the vault fold watermark (folded-to offset and last fold time), and watcher state.

Rules:

- Never print the raw output verbatim. Summarize.
- If any field looks like a secret (key, token, password, cookie, auth header), replace its value with `[REDACTED]` before showing it.
- If `mom` is not on PATH, say MOM is not installed and stop.
- If the directory is unbound, say so and mention `/mom-project` binds it — nothing is captured until then.

## Postflight (version hint)

Any `mom ...` command may print a banner to stderr like:

```
MOM 0.40.1 available. Run `brew upgrade mom` or `mom self-update`
```

If you see that line, finish the task first, then add one short line at the end of your reply suggesting the upgrade. Do not run the upgrade yourself.
