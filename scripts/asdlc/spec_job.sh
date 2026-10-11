#!/usr/bin/env bash
# The `specs` gate, shared by .github/workflows/asdlc-gates.yml and scripts/asdlc/ci_local.sh.
#   spec_job.sh <base-ref-or-sha>        env: INCLUDE_UNTRACKED=1 (local: also scan untracked files)
# For every OpenSpec change touched vs base: openspec validate, spec_lint, spec_sync. spec_sync runs
# with --strict only when the change is finished: every task in tasks.md is checked, or the change dir
# is being moved into openspec/changes/archive/ in this diff. Spec-only PRs (tasks still open) get a
# non-strict report. Archived changes added by the diff are checked --strict. Then cross_change_check
# for each touched plans/<id>/slices.yaml.
# Names come from PR-controlled paths: anything not matching ^[a-z0-9][a-z0-9-]*$ is skipped with a warning.
set -uo pipefail
BASE="${1:?usage: spec_job.sh <base-ref-or-sha>}"
ROOT=$(git rev-parse --show-toplevel) || exit 2
cd "$ROOT"
PY="${PY:-python3}"
NAME_RE='^[a-z0-9][a-z0-9-]*$'

files=$({ git diff --name-only "$BASE" HEAD 2>/dev/null || git diff --name-only "$BASE" 2>/dev/null; \
          [ "${INCLUDE_UNTRACKED:-0}" = 1 ] && { git diff --name-only "$BASE" 2>/dev/null; git ls-files --others --exclude-standard; }; } | sort -u)

safe() { [[ "$1" =~ $NAME_RE ]] || { echo "::warning::skipping unsafe name '$1'" >&2; return 1; }; }
all_checked() { # tasks.md exists, has tasks, none open
  local t="openspec/changes/$1/tasks.md"
  [ -f "$t" ] && grep -qE '^[[:space:]]*-[[:space:]]*\[[xX]\]' "$t" && ! grep -qE '^[[:space:]]*-[[:space:]]*\[ \]' "$t"
}

changes=$(printf '%s\n' "$files" | sed -n 's#^openspec/changes/\([^/]*\)/.*#\1#p' | { grep -vx archive || true; } | sort -u)
archived=$(printf '%s\n' "$files" | sed -n 's#^openspec/changes/archive/\([^/]*\)/.*#\1#p' | sort -u)
slices=$(printf '%s\n' "$files" | grep -E '^plans/[^_/][^/]*/slices\.yaml$' | sort -u || true)
[ -z "$changes$slices" ] && echo "no changed changes/slices vs $BASE"
rc=0

while IFS= read -r c; do
  [ -z "$c" ] && continue
  safe "$c" || continue
  [ -d "openspec/changes/$c" ] || continue
  echo "::group::$c"
  if command -v openspec >/dev/null 2>&1; then openspec validate "$c" --strict || rc=1
  else echo "WARN openspec not installed, validate skipped"; fi
  "$PY" scripts/asdlc/spec_lint.py "openspec/changes/$c" || rc=1
  if all_checked "$c"; then mode=--strict; else mode=""; echo "tasks not all checked: spec_sync runs non-strict"; fi
  "$PY" platform-e2e/scripts/spec_sync.py "$c" $mode || rc=1
  echo "::endgroup::"
done <<< "$changes"

while IFS= read -r a; do
  [ -z "$a" ] && continue
  safe "$a" || continue
  [ -d "openspec/changes/archive/$a/specs" ] || continue
  echo "::group::archive/$a (moved to archive: strict)"
  "$PY" platform-e2e/scripts/spec_sync.py "archive/$a" --strict || rc=1
  echo "::endgroup::"
done <<< "$archived"

while IFS= read -r s; do
  [ -z "$s" ] && continue
  d=${s#plans/}; d=${d%%/*}
  safe "$d" || continue
  "$PY" scripts/asdlc/cross_change_check.py --slices "$s" || rc=1
done <<< "$slices"
exit $rc
