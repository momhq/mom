#!/usr/bin/env bash
# check-stale.sh — Detects stale KB docs that need review
# Token cost: 0 (pure script, no AI)
# Usage: check-stale.sh [--days N]
# Default: state docs > 30 days, learning docs > 90 days

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KB_DIR="$(dirname "$SCRIPT_DIR")"
DOCS_DIR="$KB_DIR/docs"

# Colors
RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
NC='\033[0m'

# Defaults
STATE_DAYS=30
LEARNING_DAYS=90

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --state-days) STATE_DAYS="$2"; shift 2 ;;
    --learning-days) LEARNING_DAYS="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

# --- Dependency check ---
if ! command -v jq &>/dev/null; then
  echo "jq is required. Install with: brew install jq" >&2
  exit 1
fi

if [[ ! -d "$DOCS_DIR" ]]; then
  echo -e "${YELLOW}⚠ No docs directory${NC}"
  exit 0
fi

now_epoch="$(date +%s)"
stale_count=0
expired_count=0

echo "Checking for stale docs (state > ${STATE_DAYS}d, learning > ${LEARNING_DAYS}d)..."
echo ""

for file in "$DOCS_DIR"/*.json; do
  [[ -f "$file" ]] || continue

  doc_id="$(jq -r '.id' "$file")"
  lifecycle="$(jq -r '.lifecycle' "$file")"
  updated="$(jq -r '.updated' "$file")"
  doc_type="$(jq -r '.type' "$file")"

  # Skip permanent docs — they don't go stale
  [[ "$lifecycle" == "permanent" ]] && continue

  # Calculate age in days
  if [[ "$(uname)" == "Darwin" ]]; then
    updated_epoch="$(date -j -f "%Y-%m-%dT%H:%M:%S" "${updated%%Z*}" "+%s" 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S%z" "${updated}" "+%s" 2>/dev/null || echo 0)"
  else
    updated_epoch="$(date -d "$updated" +%s 2>/dev/null || echo 0)"
  fi

  if [[ "$updated_epoch" -eq 0 ]]; then
    echo -e "${YELLOW}⚠ $doc_id: could not parse date '$updated'${NC}"
    continue
  fi

  age_days=$(( (now_epoch - updated_epoch) / 86400 ))

  # Check staleness thresholds
  is_stale=false
  threshold=0
  if [[ "$lifecycle" == "state" && $age_days -gt $STATE_DAYS ]]; then
    is_stale=true
    threshold=$STATE_DAYS
  elif [[ "$lifecycle" == "learning" && $age_days -gt $LEARNING_DAYS ]]; then
    is_stale=true
    threshold=$LEARNING_DAYS
  fi

  if [[ "$is_stale" == "true" ]]; then
    echo -e "${YELLOW}STALE${NC} $doc_id (type: $doc_type, lifecycle: $lifecycle, ${age_days}d old, threshold: ${threshold}d)"
    ((stale_count++))
  fi

  # Check expired facts
  if [[ "$doc_type" == "fact" ]]; then
    expires="$(jq -r '.content.expires // empty' "$file")"
    if [[ -n "$expires" ]]; then
      if [[ "$(uname)" == "Darwin" ]]; then
        expires_epoch="$(date -j -f "%Y-%m-%dT%H:%M:%S" "${expires%%Z*}" "+%s" 2>/dev/null || echo 0)"
      else
        expires_epoch="$(date -d "$expires" +%s 2>/dev/null || echo 0)"
      fi
      if [[ "$expires_epoch" -gt 0 && "$now_epoch" -gt "$expires_epoch" ]]; then
        echo -e "${RED}EXPIRED${NC} $doc_id (expired: $expires)"
        ((expired_count++))
      fi
    fi
  fi
done

echo ""
if [[ $stale_count -eq 0 && $expired_count -eq 0 ]]; then
  echo -e "${GREEN}✓ No stale or expired docs${NC}"
else
  [[ $stale_count -gt 0 ]] && echo -e "${YELLOW}⚠ $stale_count stale doc(s) need review${NC}"
  [[ $expired_count -gt 0 ]] && echo -e "${RED}✗ $expired_count expired fact(s) need re-validation${NC}"
fi
