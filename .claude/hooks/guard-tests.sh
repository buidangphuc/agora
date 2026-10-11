#!/usr/bin/env bash
# PreToolUse(Edit|Write|MultiEdit|NotebookEdit|Bash) guard: ASDLC v2 — the
# `implementer` subagent must never modify tests; only test-author / e2e-author do.
# The payload's `agent_type` is the subagent name when the call comes from a subagent.
# Any other agent (or the main session) is not restricted. Exit 2 blocks the tool.
# Bash is best-effort: it catches obvious writes (>, >>, tee, sed -i, mv, cp, rm,
# install, truncate) aimed at a test path, not every possible indirection.
set -euo pipefail

payload="$(cat)"
agent="$(printf '%s' "$payload" | jq -r '.agent_type // empty' 2>/dev/null || true)"
[ "$agent" = "implementer" ] || exit 0

tool="$(printf '%s' "$payload" | jq -r '.tool_name // empty')"

# Test paths (ERE): *_test.go *.spec.ts *.test.ts test_*.py *_test.py */tests/* *.feature platform-e2e/**
T='(_test\.go|\.spec\.ts|\.test\.ts|(^|/)test_[^/[:space:]]*\.py|_test\.py|(^|/)tests/|\.feature|(^|/)platform-e2e/)'

block() {
  echo "BLOCKED: the implementer must not modify tests ($1)." >&2
  echo "Tests belong to test-author / e2e-author. If a test looks wrong, stop and report it as OPEN_ISSUES (STATUS: test-defect)." >&2
  exit 2
}

case "$tool" in
  Edit|Write|MultiEdit|NotebookEdit)
    f="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // .tool_input.notebook_path // empty')"
    [ -z "$f" ] && exit 0
    printf '%s' "$f" | grep -Eq "$T" && block "$f"
    ;;
  Bash)
    cmd="$(printf '%s' "$payload" | jq -r '.tool_input.command // empty')"
    [ -z "$cmd" ] && exit 0
    # redirection whose target is a test path
    printf '%s' "$cmd" | grep -Eq ">>?[[:space:]]*[\"']?[^[:space:]|;&\"']*$T" && block "redirection to a test path"
    # write-verbs with a test path among their arguments
    printf '%s' "$cmd" | grep -Eq "(^|[;&|(]|[[:space:]])(tee|mv|cp|rm|install|truncate|sed[[:space:]]+([^;&|]*[[:space:]])?-[a-zA-Z]*i[^[:space:]]*)[[:space:]][^;&|]*$T" && block "write command on a test path"
    ;;
esac
exit 0
