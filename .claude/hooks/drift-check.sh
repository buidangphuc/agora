#!/usr/bin/env bash
# PostToolUse(Edit|Write): when a coherence-anchor file changes (compose, the
# AGENTS.md port table, a .proto, or a buf.gen.yaml), run repo_doctor and speak
# ONLY if it finds drift. Silent when clean, so it never nags. Advisory, never
# blocks the edit: ❌ (runtime-breaking) → stderr + exit 2 (Claude sees + acts);
# ⚠️ (doc/wiring drift) → stdout + exit 0.
set -uo pipefail

payload="$(cat)"
f="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
[ -z "$f" ] && exit 0

case "$f" in
  *docker-compose.services.yaml|*/AGENTS.md|*.proto|*buf.gen.yaml) ;;
  *) exit 0 ;;
esac

root="${CLAUDE_PROJECT_DIR:-.}"
out="$(cd "$root" && python scripts/repo_doctor.py --root . 2>/dev/null)" || true

if printf '%s\n' "$out" | grep -q '❌'; then
  printf 'repo-doctor found drift after editing %s:\n%s\n' "$f" "$out" >&2
  exit 2
elif printf '%s\n' "$out" | grep -q '⚠️'; then
  printf 'repo-doctor (advisory) after editing %s:\n%s\n' "$f" "$out"
fi
exit 0
