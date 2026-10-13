#!/usr/bin/env bash
# ASDLC gate W1-4: red-first evidence.
#   red_first.sh [--base REF] [--head REF] [--dry-run] [--only go,python,ts] [--allow-empty] [--allow-dirty]
# unit/integration: tests added/changed in base..head are run at BASE (new tests copied into a
#   temp worktree of base) and must FAIL (compile error counts), then at HEAD and must PASS.
# e2e (platform-e2e/**): first commit touching e2e files must precede the first commit touching
#   implementation files (non-test, outside platform-e2e) of the same range.
# exit 1: a test is green at base (written after code / not testing new behaviour) or red at head,
#         or e2e written after implementation.
# --only KINDS: run only these test kinds (comma list of go|python|ts); other kinds are reported SKIP.
#   CI runs --only go,python: ts deps (node_modules) are not installed there.
# exit 1 also when: the working tree has uncommitted test/code changes (they are NOT in base..head;
#   --allow-dirty to ignore), or impl files changed but no tests did (--allow-empty to exit 0).
# exempt: a commit in base..head may carry trailer lines `Red-First-Exempt: <glob> -- <reason>` for tests
#   changed only to follow a dependency/tooling bump (no new behaviour). Matching files skip the base run
#   but must still be green at head; they are reported EXEMPT with the reason. A reason is required.
# generated code: gitignored proto output is rebuilt with `buf generate` in both worktrees for each
#   module (nearest buf.gen.yaml) owning a changed go/ts test; needs buf on PATH.
# env: REDFIRST_PYTHON (default platform-e2e/.venv/bin/python, else python3), REDFIRST_TMP (scratch dir)
set -uo pipefail

BASE="" HEAD_REF="HEAD" DRY=0 ONLY="" ALLOW_EMPTY=0 ALLOW_DIRTY="${REDFIRST_ALLOW_DIRTY:-0}"
while [ $# -gt 0 ]; do
  case "$1" in
    --base) BASE="$2"; shift 2 ;;
    --head) HEAD_REF="$2"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    --only) ONLY="$2"; shift 2 ;;
    --allow-empty) ALLOW_EMPTY=1; shift ;;
    --allow-dirty) ALLOW_DIRTY=1; shift ;;
    -h|--help) sed -n 2,20p "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

ROOT=$(git rev-parse --show-toplevel) || exit 2
cd "$ROOT"
if [ -z "$BASE" ]; then
  for m in main origin/main master; do
    if git rev-parse --verify -q "$m" >/dev/null; then BASE=$(git merge-base "$m" "$HEAD_REF") && break; fi
  done
fi
[ -n "$BASE" ] || { echo "cannot determine base (pass --base)" >&2; exit 2; }
BASE=$(git rev-parse "$BASE"); HEADSHA=$(git rev-parse "$HEAD_REF")

PY="${REDFIRST_PYTHON:-}"
[ -z "$PY" ] && { [ -x "$ROOT/platform-e2e/.venv/bin/python" ] && PY="$ROOT/platform-e2e/.venv/bin/python" || PY=python3; }

# ---- classify changed files ----
is_go_test()  { [[ "$1" == *_test.go ]]; }
is_py_test()  { [[ "$1" != platform-e2e/* ]] && [[ "$(basename "$1")" == test_*.py || "$1" == *_test.py ]]; }
is_ts_test()  { [[ "$1" != platform-e2e/* && "$1" != */e2e/* && "$1" != e2e/* ]] && [[ "$1" == *.test.ts || "$1" == *.spec.ts || "$1" == *.test.tsx || "$1" == *.spec.tsx ]]; }
is_any_test() { is_go_test "$1" || is_py_test "$1" || is_ts_test "$1" || [[ "$1" == tests/* || "$1" == test/* || "$1" == testdata/* || "$1" == fixtures/* || "$1" == */tests/* || "$1" == */test/* || "$1" == */__tests__/* || "$1" == */testdata/* || "$1" == */fixtures/* || "$(basename "$1")" == conftest.py || "$1" == *.spec.ts || "$1" == *.test.ts ]]; }
is_fixture()  { [[ "$1" == fixtures/* || "$1" == */fixtures/* || "$1" == testdata/* || "$1" == */testdata/* ]]; }  # data, never run
is_nonimpl()  { [[ "$1" == *.md || "$1" == openspec/* || "$1" == docs/* || "$1" == plans/* || "$1" == metrics/* || "$1" == scripts/asdlc/* || "$1" == gates/* || "$1" == .github/* || "$1" == platform-e2e/* || "$1" == */FEATURES.yaml || "$1" == FEATURES.yaml ]]; }

# does a kind run? (--only unset = all)
kind_on() { [ -z "$ONLY" ] || [[ ",$ONLY," == *",$1,"* ]]; }
if [ -n "$ONLY" ]; then
  for k in ${ONLY//,/ }; do case "$k" in go|python|ts) ;; *) echo "--only: unknown kind '$k' (go|python|ts)" >&2; exit 2;; esac; done
fi

CHANGED=$(git diff --name-only --diff-filter=ACMR "$BASE" "$HEADSHA")
GO_T=() PY_T=() TS_T=() E2E=() IMPL=() SUPPORT=()
while IFS= read -r f; do
  [ -z "$f" ] && continue
  if [[ "$f" == platform-e2e/* ]]; then
    # e2e tests only; platform-e2e/scripts/** is tooling, not a test
    [[ "$f" == platform-e2e/tests/* || "$f" == *.feature ]] && E2E+=("$f")
    continue
  fi
  if   is_fixture "$f"; then SUPPORT+=("$f")
  elif is_go_test "$f"; then GO_T+=("$f")
  elif is_py_test "$f"; then PY_T+=("$f")
  elif is_ts_test "$f"; then TS_T+=("$f")
  elif is_any_test "$f"; then SUPPORT+=("$f")   # conftest.py, tests/ helpers, fixtures, testdata
  elif ! is_nonimpl "$f"; then IMPL+=("$f")
  fi
done <<< "$CHANGED"

echo "red_first: base=${BASE:0:10} head=${HEADSHA:0:10}  go=${#GO_T[@]} py=${#PY_T[@]} ts=${#TS_T[@]} e2e=${#E2E[@]} support=${#SUPPORT[@]} impl=${#IMPL[@]}${ONLY:+ only=$ONLY}"
NTESTS=$(( ${#GO_T[@]} + ${#PY_T[@]} + ${#TS_T[@]} + ${#E2E[@]} ))

# Uncommitted test/code changes are invisible to base..head: refuse rather than pass on stale evidence.
if [ "$HEADSHA" = "$(git rev-parse HEAD)" ] && [ "$ALLOW_DIRTY" != 1 ] && [ "$DRY" != 1 ]; then
  DIRTY=()
  while IFS= read -r f; do
    [ -z "$f" ] && continue
    if [[ "$f" == platform-e2e/* ]] || is_any_test "$f" || ! is_nonimpl "$f"; then DIRTY+=("$f"); fi
  done < <(git status --porcelain --untracked-files=all | sed -E 's/^.{3}//; s/^.* -> //; s/^"(.*)"$/\1/')
  if [ ${#DIRTY[@]} -gt 0 ]; then
    echo "red_first: FAIL: ${#DIRTY[@]} uncommitted test/code change(s) not covered by base..head (first: ${DIRTY[0]}). Commit them, or pass --allow-dirty." >&2
    exit 1
  fi
fi
if [ ${#IMPL[@]} -gt 0 ] && [ "$NTESTS" -eq 0 ] && [ "$ALLOW_EMPTY" != 1 ]; then
  echo "red_first: FAIL: ${#IMPL[@]} implementation file(s) changed (first: ${IMPL[0]}) but no tests changed in base..head. Add tests first, or pass --allow-empty." >&2
  exit 1
fi
if [ -n "$ONLY" ]; then
  kind_on go     || { [ ${#GO_T[@]} -gt 0 ] && echo "red_first: SKIP go tests (${#GO_T[@]}): not in --only"; GO_T=(); }
  kind_on python || { [ ${#PY_T[@]} -gt 0 ] && echo "red_first: SKIP python tests (${#PY_T[@]}): not in --only"; PY_T=(); }
  kind_on ts     || { [ ${#TS_T[@]} -gt 0 ] && echo "red_first: SKIP ts tests (${#TS_T[@]}): not in --only (deps not installed here; run locally)"; TS_T=(); }
fi

go_names() { git show "$HEADSHA:$1" | grep -oE '^func (Test[A-Za-z0-9_]*)\(' | sed -E 's/^func (.*)\(/\1/' | paste -sd'|' -; }
nearest_up() { # nearest_up <worktree> <relfile> <marker>  -> dir (relative) containing marker
  local d; d=$(dirname "$2")
  while :; do
    if [ -e "$1/$d/$3" ]; then echo "$d"; return 0; fi
    [ "$d" = "." ] && return 1
    d=$(dirname "$d")
  done
}

if [ "$DRY" = 1 ]; then
  for f in "${GO_T[@]-}"; do [ -n "$f" ] && echo "WOULD RUN go: $f -> go test ./$(dirname "$f")/ -run '^($(go_names "$f"))\$' @base(expect FAIL) @head(expect PASS)"; done
  for f in "${PY_T[@]-}"; do [ -n "$f" ] && echo "WOULD RUN pytest: $f @base(expect FAIL) @head(expect PASS)"; done
  for f in "${TS_T[@]-}"; do [ -n "$f" ] && echo "WOULD RUN ts: $f @base(expect FAIL) @head(expect PASS)"; done
  if [ ${#E2E[@]} -gt 0 ]; then echo "WOULD CHECK commit order: e2e files(${#E2E[@]}) before impl files(${#IMPL[@]})"; for f in "${E2E[@]}"; do echo "  e2e: $f"; done; fi
  exit 0
fi

SCRATCH=$(mktemp -d "${REDFIRST_TMP:-${TMPDIR:-/tmp}}/redfirst.XXXXXX")
WT_BASE="$SCRATCH/base" WT_HEAD="$SCRATCH/head"
cleanup() {
  git -C "$ROOT" worktree remove --force "$WT_BASE" >/dev/null 2>&1
  git -C "$ROOT" worktree remove --force "$WT_HEAD" >/dev/null 2>&1
  git -C "$ROOT" worktree prune >/dev/null 2>&1
  rm -rf "$SCRATCH"
}
trap cleanup EXIT

FAIL=0
RESULTS=()
# Red-First-Exempt trailers: "<glob>|<reason>" per line
EXEMPT=$(git log --format=%B "$BASE..$HEADSHA" | sed -nE 's/^Red-First-Exempt:[[:space:]]*([^[:space:]]+)[[:space:]]+--[[:space:]]+(.+)$/\1|\2/p')
exempt_reason() { # exempt_reason <relfile> -> prints reason, exit 0 if exempt
  local line pat
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    pat="${line%%|*}"
    # shellcheck disable=SC2053  # glob match is intended
    if [[ "$1" == $pat ]]; then echo "${line#*|}"; return 0; fi
  done <<< "$EXEMPT"
  return 1
}
gen_proto() { # gen_proto <worktree> <moddir>: `buf generate`; buf.gen.local.yaml when its local plugins exist
  local wt="$1" mod="$2" tmpl=buf.gen.yaml p ok=1
  [ -f "$wt/$mod/buf.gen.yaml" ] || return 0
  command -v buf >/dev/null || { echo "red_first: WARN buf not installed; $mod has no generated code"; return 0; }
  if [ -f "$wt/$mod/buf.gen.local.yaml" ]; then
    for p in $(sed -nE 's/^[[:space:]]*-?[[:space:]]*local:[[:space:]]*([^[:space:]]+).*/\1/p' "$wt/$mod/buf.gen.local.yaml"); do
      command -v "$p" >/dev/null || ok=0
    done
    [ $ok = 1 ] && tmpl=buf.gen.local.yaml
  fi
  (cd "$wt/$mod" && buf generate --template "$tmpl" >"$SCRATCH/buf.log" 2>&1) ||
    echo "red_first: WARN buf generate failed in $mod ($(basename "$wt")): $(tail -1 "$SCRATCH/buf.log")"
}
GEN_MODS=()
if [ $(( ${#GO_T[@]} + ${#PY_T[@]} + ${#TS_T[@]} )) -gt 0 ]; then
  git worktree add --detach -q "$WT_BASE" "$BASE" || exit 2
  git worktree add --detach -q "$WT_HEAD" "$HEADSHA" || exit 2
  # copy ONLY the new/changed test files and test-support files (conftest, tests/, fixtures, testdata)
  for f in "${GO_T[@]-}" "${PY_T[@]-}" "${TS_T[@]-}" "${SUPPORT[@]-}"; do
    [ -z "$f" ] && continue
    mkdir -p "$WT_BASE/$(dirname "$f")"
    git show "$HEADSHA:$f" > "$WT_BASE/$f"
  done
  # generated proto code is gitignored: regenerate it in every module owning a changed go/ts test
  for f in "${GO_T[@]-}" "${TS_T[@]-}"; do
    [ -z "$f" ] && continue
    mod=$(nearest_up "$WT_HEAD" "$f" buf.gen.yaml) || continue
    [[ " ${GEN_MODS[*]-} " == *" $mod "* ]] && continue
    GEN_MODS+=("$mod")
    gen_proto "$WT_BASE" "$mod"; gen_proto "$WT_HEAD" "$mod"
  done
fi

run_test() { # run_test <worktree> <kind> <relfile> -> exit status of the test command
  local wt="$1" kind="$2" f="$3" mod rel
  case "$kind" in
    go)
      mod=$(nearest_up "$wt" "$f" go.mod) || return 97
      rel=$(dirname "${f#"$mod"/}"); [ "$mod" = "." ] && rel=$(dirname "$f")
      local names; names=$(go_names "$f"); [ -z "$names" ] && return 98
      (cd "$wt/$mod" && go test -count=1 "./$rel/" -run "^($names)\$" >"$SCRATCH/out.log" 2>&1) ;;
    py)
      mod=$(nearest_up "$wt" "$f" pyproject.toml) || mod=$(dirname "$f")
      # Services whose gRPC stubs are generated, not committed (team-ai): build them once per worktree.
      if [ -f "$wt/$mod/scripts/gen_proto.py" ] && [ ! -e "$wt/$mod/.redfirst-proto" ]; then
        (cd "$wt/$mod" && "$PY" -m scripts.gen_proto >"$SCRATCH/proto.log" 2>&1) && touch "$wt/$mod/.redfirst-proto"
      fi
      (cd "$wt/$mod" && "$PY" -m pytest -q -x -p no:cacheprovider "${f#"$mod"/}" >"$SCRATCH/out.log" 2>&1) ;;
    ts)
      mod=$(nearest_up "$wt" "$f" package.json) || return 97
      [ -d "$ROOT/$mod/node_modules" ] && [ ! -e "$wt/$mod/node_modules" ] && ln -s "$ROOT/$mod/node_modules" "$wt/$mod/node_modules"
      local runner=vitest; grep -q '"jest"' "$wt/$mod/package.json" && ! grep -q '"vitest"' "$wt/$mod/package.json" && runner=jest
      if [ $runner = vitest ]; then (cd "$wt/$mod" && npx --no-install vitest run "${f#"$mod"/}" >"$SCRATCH/out.log" 2>&1)
      else (cd "$wt/$mod" && npx --no-install jest "${f#"$mod"/}" >"$SCRATCH/out.log" 2>&1); fi ;;
  esac
}

check_file() { # check_file <kind> <relfile>
  local kind="$1" f="$2" rb rh red green verdict why
  if why=$(exempt_reason "$f"); then
    run_test "$WT_HEAD" "$kind" "$f"; rh=$?
    if [ $rh -eq 98 ]; then RESULTS+=("$f|n/a|n/a|SKIP (no Test funcs)"); return; fi
    if [ $rh -eq 0 ]; then verdict="EXEMPT ($why)"; green=yes; else verdict="FAIL: red at head"; green=no; FAIL=1; fi
    RESULTS+=("$f|red@base=skip|green@head=$green|$verdict"); return
  fi
  run_test "$WT_BASE" "$kind" "$f"; rb=$?
  if [ $rb -eq 98 ]; then RESULTS+=("$f|n/a|n/a|SKIP (no Test funcs)"); return; fi
  run_test "$WT_HEAD" "$kind" "$f"; rh=$?
  [ $rb -ne 0 ] && red=yes || red=no
  [ $rh -eq 0 ] && green=yes || green=no
  if [ $red = yes ] && [ $green = yes ]; then verdict=OK
  elif [ $red = no ]; then verdict="FAIL: green at base (test written after code / not testing new behaviour)"; FAIL=1
  else verdict="FAIL: red at head"; FAIL=1; fi
  RESULTS+=("$f|red@base=$red|green@head=$green|$verdict")
}

for f in "${GO_T[@]-}"; do [ -n "$f" ] && check_file go "$f"; done
for f in "${PY_T[@]-}"; do [ -n "$f" ] && check_file py "$f"; done
for f in "${TS_T[@]-}"; do [ -n "$f" ] && check_file ts "$f"; done

# ---- e2e commit order ----
if [ ${#E2E[@]} -gt 0 ]; then
  ORDER=$(git rev-list --reverse "$BASE..$HEADSHA")
  idx() { echo "$ORDER" | grep -n "^$1\$" | cut -d: -f1; }
  first_e2e=$(git log --reverse --format=%H "$BASE..$HEADSHA" -- "${E2E[@]}" | head -1)
  if [ ${#IMPL[@]} -eq 0 ]; then
    RESULTS+=("e2e(${#E2E[@]} files)|first-e2e=$(idx "$first_e2e")|first-impl=none|OK (no implementation files in range)")
  else
    first_impl=$(git log --reverse --format=%H "$BASE..$HEADSHA" -- "${IMPL[@]}" | head -1)
    ie=$(idx "$first_e2e"); ii=$(idx "$first_impl")
    if [ "$ie" -lt "$ii" ]; then v=OK
    else v="FAIL: first impl commit (${first_impl:0:8}) is not after first e2e commit (${first_e2e:0:8})"; FAIL=1; fi
    RESULTS+=("e2e(${#E2E[@]} files)|first-e2e=#$ie|first-impl=#$ii|$v")
  fi
fi

for r in "${RESULTS[@]-}"; do [ -n "$r" ] && echo "$r" | awk -F'|' '{printf "%-70s %-14s %-16s %s\n",$1,$2,$3,$4}'; done
[ ${#RESULTS[@]} -eq 0 ] && echo "red_first: no tests changed in range"
[ $FAIL -eq 0 ] && echo "red_first: PASS" || echo "red_first: FAIL"
exit $FAIL
