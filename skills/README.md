<div align="center">

# MOM Skills for Claude Code

User-invocable skills that expose MOM’s CLI-first memory workflows.

</div>

## What’s included

This plugin provides 5 skills:

- `/mom:mom-status` — check MOM health, Ledger stats, and vault watermark (sanitized summary)
- `/mom:mom-recall <topic>` — recall past decisions by navigating the markdown vault
- `/mom:mom-project` — bind the current directory to a MOM project id for scoped memory
- `/mom:mom-fold` — fold newly captured sessions into the markdown vault (end-of-session save)
- `/mom:mom-rebuild` — rebuild the vault from scratch over the full captured history


## Install

From your project root:

```bash
# install from registry/repo source
/plugin install momhq/mom
```

Or test locally during development:

```bash
claude --plugin-dir ./skills
```

Then reload plugins in Claude Code:

```text
/reload-plugins
```

## Usage examples

```text
/mom:mom-status
/mom:mom-recall decision about auth boundary
/mom:mom-project
/mom:mom-fold
/mom:mom-rebuild
```

## Plugin layout

```text
skills/
├── .claude-plugin/plugin.json
├── mom-status/SKILL.md
├── mom-recall/SKILL.md
├── mom-project/SKILL.md
├── mom-fold/SKILL.md
└── mom-rebuild/SKILL.md
```

## Behavior and safety

- `mom-status` returns a concise parsed summary and avoids raw verbatim dumps
- Sensitive fields should be redacted if ever present (`[REDACTED]`)
- `mom-project` requires explicit user approval before writing `.mom-project.yaml`
- `mom-recall` reads only the markdown vault — it never invents past decisions
- `mom-rebuild` requires explicit user approval before regenerating the vault
