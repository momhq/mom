#!/usr/bin/env bash
# build-index.sh — Rebuilds index.json from all KB docs
# Token cost: 0 (pure script, no AI)
# Usage: build-index.sh
# Called by: SessionEnd hook, or manually

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KB_DIR="$(dirname "$SCRIPT_DIR")"
DOCS_DIR="$KB_DIR/docs"
INDEX_FILE="$KB_DIR/index.json"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

ok()   { echo -e "${GREEN}✓ $1${NC}" >&2; }
warn() { echo -e "${YELLOW}⚠ $1${NC}" >&2; }

# --- Dependency check ---
if ! command -v jq &>/dev/null; then
  echo "jq is required but not installed. Install with: brew install jq" >&2
  exit 1
fi

# --- Build index ---
if [[ ! -d "$DOCS_DIR" ]]; then
  warn "No docs directory found at $DOCS_DIR"
  exit 0
fi

doc_files=("$DOCS_DIR"/*.json)

# Handle empty docs directory
if [[ ! -f "${doc_files[0]}" ]]; then
  warn "No docs found in $DOCS_DIR — writing empty index"
  cat > "$INDEX_FILE" <<'EOF'
{
  "version": "1",
  "last_rebuilt": "",
  "stats": {
    "total_docs": 0,
    "total_tags": 0,
    "docs_by_type": {},
    "stale_count": 0,
    "most_connected_tag": ""
  },
  "by_tag": {},
  "by_type": {},
  "by_scope": {},
  "by_lifecycle": {}
}
EOF
  # Fill in timestamp
  local_ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  jq --arg ts "$local_ts" '.last_rebuilt = $ts' "$INDEX_FILE" > "$INDEX_FILE.tmp" && mv "$INDEX_FILE.tmp" "$INDEX_FILE"
  exit 0
fi

# Collect all docs into a temporary merged array
merged="[]"
for file in "${doc_files[@]}"; do
  [[ -f "$file" ]] || continue
  merged="$(echo "$merged" | jq --slurpfile doc "$file" '. + $doc')"
done

# Generate the index
now="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

index="$(echo "$merged" | jq --arg now "$now" '
  # by_tag: { tag: [doc_ids] }
  def by_tag:
    reduce .[] as $doc ({};
      reduce $doc.tags[] as $tag (.;
        .[$tag] = (.[$tag] // []) + [$doc.id]
      )
    );

  # by_type: { type: [doc_ids] }
  def by_type:
    reduce .[] as $doc ({};
      .[$doc.type] = (.[$doc.type] // []) + [$doc.id]
    );

  # by_scope: { scope: [doc_ids] }
  def by_scope:
    reduce .[] as $doc ({};
      .[$doc.scope] = (.[$doc.scope] // []) + [$doc.id]
    );

  # by_lifecycle: { lifecycle: [doc_ids] }
  def by_lifecycle:
    reduce .[] as $doc ({};
      .[$doc.lifecycle] = (.[$doc.lifecycle] // []) + [$doc.id]
    );

  # stale: state docs not updated in 30+ days
  def stale_count:
    [.[] | select(
      .lifecycle == "state" and
      (($now | split("T")[0] | split("-") | map(tonumber)) as $n |
       (.updated | split("T")[0] | split("-") | map(tonumber)) as $u |
       (($n[0] - $u[0]) * 365 + ($n[1] - $u[1]) * 30 + ($n[2] - $u[2])) > 30)
    )] | length;

  # most_connected_tag
  def most_connected:
    by_tag | to_entries | sort_by(.value | length) | last // {key: ""} | .key;

  # all unique tags
  def all_tags:
    [.[].tags[]] | unique;

  # docs_by_type counts
  def type_counts:
    reduce .[] as $doc ({};
      .[$doc.type] = ((.[$doc.type] // 0) + 1)
    );

  {
    version: "1",
    last_rebuilt: $now,
    stats: {
      total_docs: length,
      total_tags: (all_tags | length),
      docs_by_type: type_counts,
      stale_count: stale_count,
      most_connected_tag: most_connected
    },
    by_tag: by_tag,
    by_type: by_type,
    by_scope: by_scope,
    by_lifecycle: by_lifecycle
  }
')"

echo "$index" > "$INDEX_FILE"

total="$(echo "$index" | jq '.stats.total_docs')"
tags="$(echo "$index" | jq '.stats.total_tags')"
stale="$(echo "$index" | jq '.stats.stale_count')"

ok "Index rebuilt: $total docs, $tags tags, $stale stale"
