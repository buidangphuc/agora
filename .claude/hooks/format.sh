#!/usr/bin/env bash
# PostToolUse(Edit|Write) auto-format. Self-activating and non-fatal:
# each formatter runs only if it is on PATH, so this no-ops cleanly when the
# Go toolchain / prettier are absent (e.g. formatting done inside Docker).
set -uo pipefail

payload="$(cat)"
f="$(printf '%s' "$payload" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
[ -z "$f" ] && exit 0
[ -f "$f" ] || exit 0

case "$f" in
  *.go)
    command -v gofmt    >/dev/null 2>&1 && gofmt -w "$f"    || true
    command -v goimports >/dev/null 2>&1 && goimports -w "$f" || true
    ;;
  *.ts|*.tsx|*.js|*.jsx|*.css|*.json)
    # Best-effort: use the frontend's prettier if the file lives there.
    case "$f" in
      *team-frontend*)
        dir="${f%%/team-frontend/*}/team-frontend"
        if [ -x "$dir/node_modules/.bin/prettier" ]; then
          "$dir/node_modules/.bin/prettier" --write "$f" >/dev/null 2>&1 || true
        fi
        ;;
    esac
    ;;
  *.py)
    command -v ruff >/dev/null 2>&1 && ruff format "$f" >/dev/null 2>&1 || true
    ;;
esac
exit 0
