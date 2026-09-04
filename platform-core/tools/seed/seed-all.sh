#!/usr/bin/env bash
# ==============================================================================
# MASTER SEED ORCHESTRATOR — MODULAR SHOPEE SEED PIPELINE
# ==============================================================================
# Usage:
#   ./tools/seed/seed-all.sh          # Run all modules sequentially
#   ./tools/seed/01-base-info.sh      # Run only base-info
#   ./tools/seed/02-vouchers.sh       # Run only vouchers
#   ./tools/seed/03-flashsale.sh      # Run only flashsale
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"

echo "======================================================================"
echo "🚀 EXECUTING MODULAR SEED PIPELINE (BASE-INFO -> VOUCHERS -> FLASHSALE)"
echo "======================================================================"

chmod +x "$SCRIPT_DIR"/01-base-info.sh "$SCRIPT_DIR"/02-vouchers.sh "$SCRIPT_DIR"/03-flashsale.sh

# 1. Base Info
"$SCRIPT_DIR/01-base-info.sh"
echo ""

# 2. Vouchers
"$SCRIPT_DIR/02-vouchers.sh"
echo ""

# 3. Flash Sale
"$SCRIPT_DIR/03-flashsale.sh"
echo ""

echo "======================================================================"
echo "🎉 ALL SEED TOPICS APPLIED SUCCESSFULLY!"
echo "======================================================================"
