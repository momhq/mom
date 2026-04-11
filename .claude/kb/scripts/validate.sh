#!/usr/bin/env bash
# validate.sh — Validates a KB doc against schema.json
# Token cost: 0 (pure script, no AI)
# Usage: validate.sh <doc.json>
# Exit codes: 0 = valid, 1 = error, 2 = invalid (blocks the write via hook)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KB_DIR="$(dirname "$SCRIPT_DIR")"
SCHEMA="$KB_DIR/schema.json"
DOCS_DIR="$KB_DIR/docs"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

error() { echo -e "${RED}✗ $1${NC}" >&2; }
warn()  { echo -e "${YELLOW}⚠ $1${NC}" >&2; }
ok()    { echo -e "${GREEN}✓ $1${NC}" >&2; }

# --- Dependency check ---
check_deps() {
  if ! command -v jq &>/dev/null; then
    error "jq is required but not installed. Install with: brew install jq"
    exit 1
  fi
}

# --- Validate a single doc ---
validate_doc() {
  local file="$1"
  local filename
  filename="$(basename "$file" .json)"
  local errors=0

  # 1. Valid JSON?
  if ! jq empty "$file" 2>/dev/null; then
    error "$filename: invalid JSON"
    return 2
  fi

  # 2. Required base fields
  local required_fields=("id" "type" "lifecycle" "scope" "tags" "created" "created_by" "updated" "updated_by" "content")
  for field in "${required_fields[@]}"; do
    if [[ "$(jq -r "has(\"$field\")" "$file")" != "true" ]]; then
      error "$filename: missing required field '$field'"
      ((errors++))
    fi
  done

  # Bail early if base fields missing
  if [[ $errors -gt 0 ]]; then
    return 2
  fi

  # 3. id matches filename
  local doc_id
  doc_id="$(jq -r '.id' "$file")"
  if [[ "$doc_id" != "$filename" ]]; then
    error "$filename: id '$doc_id' does not match filename"
    ((errors++))
  fi

  # 4. id is kebab-case
  if ! echo "$doc_id" | grep -qE '^[a-z0-9]+(-[a-z0-9]+)*$'; then
    error "$filename: id '$doc_id' is not valid kebab-case"
    ((errors++))
  fi

  # 5. type is valid enum
  local doc_type
  doc_type="$(jq -r '.type' "$file")"
  local valid_types=("rule" "skill" "identity" "decision" "pattern" "fact" "feedback" "reference" "metric")
  local type_valid=false
  for t in "${valid_types[@]}"; do
    if [[ "$doc_type" == "$t" ]]; then
      type_valid=true
      break
    fi
  done
  if [[ "$type_valid" == "false" ]]; then
    error "$filename: invalid type '$doc_type'"
    ((errors++))
  fi

  # 6. lifecycle is valid enum
  local lifecycle
  lifecycle="$(jq -r '.lifecycle' "$file")"
  if [[ "$lifecycle" != "permanent" && "$lifecycle" != "learning" && "$lifecycle" != "state" ]]; then
    error "$filename: invalid lifecycle '$lifecycle'"
    ((errors++))
  fi

  # 7. scope is valid enum
  local scope
  scope="$(jq -r '.scope' "$file")"
  if [[ "$scope" != "core" && "$scope" != "project" ]]; then
    error "$filename: invalid scope '$scope'"
    ((errors++))
  fi

  # 8. tags is non-empty array with kebab-case strings
  local tag_count
  tag_count="$(jq '.tags | length' "$file")"
  if [[ "$tag_count" -lt 1 ]]; then
    error "$filename: tags must have at least 1 element"
    ((errors++))
  fi

  local invalid_tags
  invalid_tags="$(jq -r '.tags[] | select(test("^[a-z0-9]+(-[a-z0-9]+)*$") | not)' "$file")"
  if [[ -n "$invalid_tags" ]]; then
    error "$filename: invalid tag(s) (must be lowercase kebab-case): $invalid_tags"
    ((errors++))
  fi

  # 9. Dates are present (format validation is basic — ISO 8601 pattern)
  local iso_pattern='^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}'
  local created
  created="$(jq -r '.created' "$file")"
  if ! echo "$created" | grep -qE "$iso_pattern"; then
    error "$filename: created '$created' is not valid ISO 8601"
    ((errors++))
  fi

  local updated
  updated="$(jq -r '.updated' "$file")"
  if ! echo "$updated" | grep -qE "$iso_pattern"; then
    error "$filename: updated '$updated' is not valid ISO 8601"
    ((errors++))
  fi

  # 10. content is an object
  local content_type
  content_type="$(jq -r '.content | type' "$file")"
  if [[ "$content_type" != "object" ]]; then
    error "$filename: content must be an object, got '$content_type'"
    ((errors++))
  fi

  # 11. Type-specific content validation
  case "$doc_type" in
    rule)
      for field in "rule" "why" "how_to_apply" "responsibility"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: rule content missing required field '$field'"
          ((errors++))
        fi
      done
      if [[ "$(jq '.content.how_to_apply | length' "$file" 2>/dev/null)" -lt 1 ]]; then
        error "$filename: rule content.how_to_apply must have at least 1 item"
        ((errors++))
      fi
      ;;
    skill)
      for field in "description" "triggers" "invoked_by" "steps"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: skill content missing required field '$field'"
          ((errors++))
        fi
      done
      ;;
    identity)
      if [[ "$(jq -r '.content | has("what")' "$file")" != "true" ]]; then
        error "$filename: identity content missing required field 'what'"
        ((errors++))
      fi
      ;;
    decision)
      for field in "decision" "context"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: decision content missing required field '$field'"
          ((errors++))
        fi
      done
      ;;
    pattern)
      for field in "pattern" "when_to_use"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: pattern content missing required field '$field'"
          ((errors++))
        fi
      done
      ;;
    fact)
      if [[ "$(jq -r '.content | has("fact")' "$file")" != "true" ]]; then
        error "$filename: fact content missing required field 'fact'"
        ((errors++))
      fi
      ;;
    feedback)
      for field in "feedback" "why" "how_to_apply"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: feedback content missing required field '$field'"
          ((errors++))
        fi
      done
      ;;
    reference)
      for field in "description" "purpose"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: reference content missing required field '$field'"
          ((errors++))
        fi
      done
      ;;
    metric)
      for field in "task_id" "manager" "review" "owner" "rework_cycles" "hiring_loop" "delegation" "internal_iterations" "leo_errors"; do
        if [[ "$(jq -r ".content | has(\"$field\")" "$file")" != "true" ]]; then
          error "$filename: metric content missing required field '$field'"
          ((errors++))
        fi
      done
      ;;
  esac

  # 12. No extra top-level fields
  local extra_fields
  extra_fields="$(jq -r 'keys[] | select(. != "id" and . != "type" and . != "lifecycle" and . != "scope" and . != "tags" and . != "created" and . != "created_by" and . != "updated" and . != "updated_by" and . != "content")' "$file")"
  if [[ -n "$extra_fields" ]]; then
    error "$filename: unexpected top-level field(s): $extra_fields"
    ((errors++))
  fi

  if [[ $errors -gt 0 ]]; then
    error "$filename: $errors error(s) found"
    return 2
  fi

  ok "$filename: valid"
  return 0
}

# --- Main ---
check_deps

if [[ $# -eq 0 ]]; then
  # Validate all docs
  total=0
  failed=0
  if [[ -d "$DOCS_DIR" ]]; then
    for file in "$DOCS_DIR"/*.json; do
      [[ -f "$file" ]] || continue
      ((total++))
      if ! validate_doc "$file"; then
        ((failed++))
      fi
    done
  fi

  if [[ $total -eq 0 ]]; then
    warn "No docs found in $DOCS_DIR"
    exit 0
  fi

  echo ""
  if [[ $failed -gt 0 ]]; then
    error "$failed/$total doc(s) failed validation"
    exit 2
  else
    ok "All $total doc(s) valid"
    exit 0
  fi
else
  # Validate specific file
  validate_doc "$1"
fi
