#!/usr/bin/env bash
# Hook: PostToolUse — validates KB doc after Write, then rebuilds index
# Runs after agent writes to .claude/kb/docs/*.json
# Token cost: 0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KB_SCRIPTS="$SCRIPT_DIR/../kb/scripts"

# The tool_input is passed via stdin as JSON
input="$(cat)"
file_path="$(echo "$input" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")"

# Only act on .claude/kb/docs/*.json files
if [[ -z "$file_path" || "$file_path" != *".claude/kb/docs/"*".json" ]]; then
  exit 0
fi

# Run validation
validation_output="$("$KB_SCRIPTS/validate.sh" "$file_path" 2>&1)" || true

if echo "$validation_output" | grep -q "✗"; then
  # Validation failed
  jq -n --arg ctx "KB validation failed for $(basename "$file_path"): $validation_output — fix the doc and re-write it." '{
    hookSpecificOutput: {
      hookEventName: "PostToolUse",
      additionalContext: $ctx
    }
  }'
  exit 0
fi

# Valid — rebuild index
"$KB_SCRIPTS/build-index.sh" >/dev/null 2>&1 || true

jq -n --arg ctx "KB doc $(basename "$file_path") validated + index rebuilt." '{
  hookSpecificOutput: {
    hookEventName: "PostToolUse",
    additionalContext: $ctx
  }
}'
exit 0
