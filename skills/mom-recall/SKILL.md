---
name: mom-recall
description: Recall what was decided, tried, preferred, or learned on this project by reading MOM's markdown vault. Use when the user asks what was decided, discussed, preferred, tried, or remembered about a topic.
user-invocable: true
allowed-tools: Read, Grep, Glob, Bash(command -v mom*), Bash(brew install momhq/tap/mom*)
argument-hint: <topic or question>
---

MOM's memory for this project lives as plain markdown under `.mom/vault/`. There is no search command — you navigate the files directly.

## Flow

1. Confirm the vault exists by checking for `.mom/vault/INDEX.md`.

   - If it exists → continue.
   - If it is missing → the vault has not been built yet. Tell the user to run `/mom-fold` (or `mom vault fold`), then stop.

2. Read `.mom/vault/INDEX.md`. It maps topics to vault files.

3. From the index, open the file(s) that match the user's topic. If the index is large or the match is unclear, grep the vault for keywords:

   ```bash
   grep -ril "<keyword>" .mom/vault/
   ```

4. Answer from what you read.

Output format when there are matches:

```text
<direct answer in 2–6 lines>

Sources:
- .mom/vault/<file>.md
```

## Rules

- Answer only from the vault files you actually read — never from prior-session memory or guesswork.
- If no vault file matches the topic, say so plainly. Do not invent past decisions.
- Cite the vault file paths you used as sources.
- This skill only reads the vault — never fold or rebuild from here.

## Postflight (version hint)

Any `mom ...` command may print a banner to stderr like:

```
MOM 0.40.1 available. Run `brew upgrade mom` or `mom self-update`
```

If you see that line, finish the task first, then add one short line at the end of your reply suggesting the upgrade. Do not run the upgrade yourself.
