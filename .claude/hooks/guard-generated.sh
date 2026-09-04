#!/usr/bin/env bash
# PreToolUse(Edit|Write) guard: enforce AGENTS.md rule #4 — never hand-edit
# generated code or fork the proto contract outside platform-core.
# Reads the hook payload as JSON on stdin. Exit 2 blocks the tool.
set -euo pipefail

payload="$(cat)"
f="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty')"
[ -z "$f" ] && exit 0

case "$f" in
  */generated/*|*.pb.go|*_connect.go|*.pb.gw.go|*_grpc.pb.go|*.connect.go)
    echo "BLOCKED: '$f' is generated from the proto contract." >&2
    echo "Edit platform-core/packages/proto and regenerate (buf generate). Never hand-edit generated code (AGENTS.md rule #4)." >&2
    exit 2
    ;;
esac
exit 0
