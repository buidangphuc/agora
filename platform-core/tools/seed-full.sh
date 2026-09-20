#!/usr/bin/env bash
# ==============================================================================
# SEED FULL MARKETPLACE SCRIPT — AGORA E-COMMERCE PLATFORM
# ==============================================================================
# Shell wrapper around platform-core/tools/seed_full_marketplace.py
#
# Usage:
#   ./platform-core/tools/seed-full.sh [--gateway http://localhost:8080] [--dry-run]
#   GATEWAY_URL=http://localhost:8080 ./platform-core/tools/seed-full.sh
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

GW="${GATEWAY_URL:-http://localhost:8080}"
PYTHON_BIN="${PYTHON_BIN:-python3}"

if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  echo "❌ Error: Python 3 ($PYTHON_BIN) is not installed or not in PATH."
  exit 1
fi

echo "======================================================================"
echo "🚀 Agora Marketplace Full Dataset Seeding"
echo "======================================================================"
echo "• Gateway Base URL:   $GW"
echo "• Seeding Tool:       $SCRIPT_DIR/seed_full_marketplace.py"
echo "• Test Data Target:   $REPO_ROOT/platform-e2e/test-data/marketplace_full_seed.json"
echo "======================================================================"

# Run the comprehensive Python seeder with all passed CLI arguments
exec "$PYTHON_BIN" "$SCRIPT_DIR/seed_full_marketplace.py" \
  --gateway "$GW" \
  --export-json "$REPO_ROOT/platform-e2e/test-data/marketplace_full_seed.json" \
  "$@"
