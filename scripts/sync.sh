#!/bin/bash
# sync.sh — idempotent sync from copilot-core to ~/.claude/
#
# Symlinks every .md file from copilot-core/agents and copilot-core/rules
# into the corresponding ~/.claude/ locations. After first run, future
# updates come automatically via `git pull` in copilot-core — symlinks
# point at live files in the repo.
#
# Re-run this script only when copilot-core topology changes (files added
# or removed). Content edits don't need a re-run.
#
# See: docs/rdds/2026-04-08-copilot-core-architecture/rdd.md §8.8 (D8)

set -e

# Resolve CORE_DIR relative to this script's location, falling back to default
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE_DIR="${CORE_DIR:-$(dirname "$SCRIPT_DIR")}"
CLAUDE_DIR="$HOME/.claude"

# --- Sanity checks ----------------------------------------------------------

if [ ! -d "$CORE_DIR" ]; then
  echo "Error: copilot-core not found at $CORE_DIR"
  echo "Clone it first: git clone <url> $CORE_DIR"
  exit 1
fi

if [ ! -d "$CORE_DIR/agents" ] || [ ! -d "$CORE_DIR/rules" ]; then
  echo "Error: $CORE_DIR doesn't look like a copilot-core checkout"
  echo "Expected: $CORE_DIR/agents and $CORE_DIR/rules"
  exit 1
fi

# --- Create target dirs -----------------------------------------------------

mkdir -p "$CLAUDE_DIR/agents" "$CLAUDE_DIR/rules"

# --- Sync agents (recursive, flat destination) ------------------------------
# Claude Code loads ~/.claude/agents/*.md flat, so we flatten subdirs like
# agents/managers/dev.md → ~/.claude/agents/dev.md

agent_count=0
while IFS= read -r -d '' src; do
  basename=$(basename "$src")
  ln -sf "$src" "$CLAUDE_DIR/agents/$basename"
  echo "  agent: $basename"
  agent_count=$((agent_count + 1))
done < <(find "$CORE_DIR/agents" -type f -name "*.md" -print0)

# --- Sync rules -------------------------------------------------------------

rule_count=0
while IFS= read -r -d '' src; do
  basename=$(basename "$src")
  ln -sf "$src" "$CLAUDE_DIR/rules/$basename"
  echo "  rule:  $basename"
  rule_count=$((rule_count + 1))
done < <(find "$CORE_DIR/rules" -type f -name "*.md" -print0)

# --- Clean up dangling symlinks --------------------------------------------
# If core removed files, the symlinks pointing to them are now broken.
# Only delete symlinks (not real files) whose target doesn't exist.

dangling_count=0
for dir in "$CLAUDE_DIR/agents" "$CLAUDE_DIR/rules"; do
  while IFS= read -r -d '' link; do
    if [ ! -e "$link" ]; then
      rm "$link"
      echo "  removed dangling: $(basename "$link")"
      dangling_count=$((dangling_count + 1))
    fi
  done < <(find "$dir" -type l -print0 2>/dev/null || true)
done

# --- Report -----------------------------------------------------------------

echo ""
echo "✓ Sync complete."
echo "  $agent_count agent(s) linked"
echo "  $rule_count rule(s) linked"
if [ "$dangling_count" -gt 0 ]; then
  echo "  $dangling_count dangling symlink(s) cleaned"
fi
echo ""
echo "Source:  $CORE_DIR"
echo "Target:  $CLAUDE_DIR"
echo ""
echo "Future content updates: cd $CORE_DIR && git pull"
echo "(re-run sync.sh only when files are added or removed from core)"
