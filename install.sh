#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI_DIR="$SCRIPT_DIR/cli"

echo "Installing copilot-core CLI..."

cd "$CLI_DIR"
npm install
npm run build
npm link

echo ""
echo "✓ copilot-core CLI installed globally."
echo "  Run 'copilot-core init' in any project to onboard it."
