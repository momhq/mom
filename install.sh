#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLI_DIR="$SCRIPT_DIR/cli"

echo "Installing LEO CLI..."

cd "$CLI_DIR"
npm install
npm run build
npm link

echo ""
echo "✓ LEO CLI installed globally."
echo "  Run 'leo init' in any project to onboard it."
echo "  Run 'leo migrate-kb' in an existing project to migrate to KB architecture."
