#!/bin/bash
# collect-metrics.sh — scan all projects for metrics, combine into dashboard/data.jsonl
# Usage: bash scripts/collect-metrics.sh [search_root]
# Default search_root: ~/Github
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT="$CORE_DIR/dashboard/data.jsonl"
SEARCH_ROOT="${1:-$HOME/Github}"

mkdir -p "$CORE_DIR/dashboard"
> "$OUTPUT"

count=0
projects=0

for metrics_dir in "$SEARCH_ROOT"/*/.claude/metrics; do
  [ -d "$metrics_dir" ] || continue
  # Extract project name from path: ~/Github/<project>/.claude/metrics → <project>
  project=$(basename "$(cd "$metrics_dir/../.." && pwd)")
  projects=$((projects + 1))

  for f in "$metrics_dir"/*.jsonl; do
    [ -f "$f" ] || continue
    month=$(basename "$f" .jsonl)
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      # Inject project and month fields at the start of JSON
      echo "$line" | sed "s/^{/{\"project\":\"$project\",\"month\":\"$month\",/" >> "$OUTPUT"
      count=$((count + 1))
    done < "$f"
  done
done

echo "Collected $count entries from $projects project(s) → $OUTPUT"
echo "Open dashboard/index.html in your browser."
