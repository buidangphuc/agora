#!/usr/bin/env bash
set -euo pipefail

# Build all Marketplace Docker images in parallel
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "🚀 Building all 12 marketplace images in parallel..."
cd "$REPO_ROOT"

docker compose -f docker-compose.services.yaml build --parallel "$@"

echo "✨ All marketplace images built successfully!"
