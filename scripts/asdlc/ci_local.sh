#!/usr/bin/env bash
# Run the asdlc-gates jobs locally, without act (mirrors .github/workflows/asdlc-gates.yml).
#   ci_local.sh [--docker-tools] [job...]      jobs: specs features repo proto tests-asdlc tdd security
# Default: all jobs except mutation/security-review (CI-only, advisory/optional).
# env: BASE (default main; PR base ref), ALLOW_DIRTY=1 (let the tdd job ignore uncommitted changes). --docker-tools runs gitleaks/trivy/semgrep via official images.
set -uo pipefail
ROOT=$(git rev-parse --show-toplevel) || exit 2
cd "$ROOT"
BASE="${BASE:-main}"
DOCKER_TOOLS=0
JOBS=()
for a in "$@"; do
  case "$a" in
    --docker-tools) DOCKER_TOOLS=1 ;;
    -h|--help) sed -n 2,5p "$0"; exit 0 ;;
    *) JOBS+=("$a") ;;
  esac
done
[ ${#JOBS[@]} -eq 0 ] && JOBS=(specs features repo proto tests-asdlc tdd security)

PY=python3
[ -x platform-e2e/.venv/bin/python ] && PY="$ROOT/platform-e2e/.venv/bin/python"
export REDFIRST_PYTHON="$PY"
BASE_SHA=$(git merge-base "$BASE" HEAD 2>/dev/null || echo "$BASE")

names=(); results=()
have() { command -v "$1" >/dev/null 2>&1; }
record() { names+=("$1"); results+=("$2"); }
# run_job name cmd... : runs in a subshell, records PASS/FAIL
run_job() {
  local name="$1"; shift
  echo "=== $name"
  ( set -e; "$@" ); local rc=$?   # plain statement: set -e stays active inside the subshell
  if [ $rc -eq 0 ]; then record "$name" PASS; else record "$name" FAIL; fi
}
skip() { echo "SKIP $1: $2"; record "$1" "SKIP ($2)"; }

changed() { git diff --name-only "$BASE_SHA" 2>/dev/null; git ls-files --others --exclude-standard; }

job_specs() {
  # shared with CI: scripts/asdlc/spec_job.sh (strict spec_sync only for finished/archived changes)
  PY="$PY" INCLUDE_UNTRACKED=1 bash scripts/asdlc/spec_job.sh "$BASE_SHA"
}
job_features() { make -C platform-e2e features-check; }
job_repo() {
  $PY scripts/repo_doctor.py
  local d
  for d in plans/*/; do
    [ "$(basename "$d")" = _template ] && continue
    [ -f "$d/slices.yaml" ] || continue
    $PY scripts/validate_plan.py "$d"
  done
  $PY scripts/asdlc_config.py --check
  $PY scripts/asdlc/check_gates.py
}
job_proto() {
  ( cd platform-core/packages/proto && buf lint \
    && buf breaking --against "$ROOT/.git#branch=$BASE,subdir=platform-core/packages/proto" )
}
job_tests_asdlc() {
  local dirs="scripts/asdlc/tests platform-e2e/scripts/tests"
  [ -d scripts/asdlc/kev_eval/tests ] && dirs="$dirs scripts/asdlc/kev_eval/tests"
  $PY -m pytest -q --ignore-glob="*/fixtures/*" $dirs
  bash .claude/hooks/tests/test_guard_tests.sh
  bash scripts/asdlc/tests/test_red_first.sh
}
job_tdd() {
  # a dirty tree is refused (uncommitted tests/code are not in base..HEAD); ALLOW_DIRTY=1 to override locally
  bash scripts/asdlc/red_first.sh --base "$BASE_SHA" ${ALLOW_DIRTY:+--allow-dirty}
  $PY scripts/asdlc/no_self_mock.py --base "$BASE"
}
job_security() {
  if [ "$DOCKER_TOOLS" -ne 1 ]; then
    for t in gitleaks trivy semgrep; do
      if have "$t"; then echo "have $t"; else echo "SKIP $t: not installed (use --docker-tools)"; fi
    done
  fi
  local rc=0
  if have gitleaks; then gitleaks detect --source . --config .gitleaks.toml --no-banner || rc=1
  elif [ "$DOCKER_TOOLS" -eq 1 ] && have docker; then
    docker run --rm -v "$ROOT:/repo" zricethezav/gitleaks:latest detect --source /repo --config /repo/.gitleaks.toml --no-banner || rc=1
  fi
  if have trivy; then trivy fs --severity HIGH,CRITICAL --ignore-unfixed --ignorefile .trivyignore.yaml --exit-code 1 . || rc=1
  elif [ "$DOCKER_TOOLS" -eq 1 ] && have docker; then
    docker run --rm -v "$ROOT:/repo" aquasec/trivy:latest fs --severity HIGH,CRITICAL --ignore-unfixed --ignorefile /repo/.trivyignore.yaml --exit-code 1 /repo || rc=1
  fi
  # semgrep is advisory in CI (continue-on-error): never fails the local job either
  if have semgrep; then semgrep scan --config p/default --metrics=off || echo "semgrep: findings (advisory)"
  elif [ "$DOCKER_TOOLS" -eq 1 ] && have docker; then
    docker run --rm -v "$ROOT:/src" semgrep/semgrep semgrep scan --config p/default --metrics=off || echo "semgrep: findings (advisory)"
  fi
  return $rc
}

for j in "${JOBS[@]}"; do
  case "$j" in
    specs) run_job specs job_specs ;;
    features) run_job features job_features ;;
    repo) run_job repo job_repo ;;
    proto) have buf && run_job proto job_proto || skip proto "buf not installed" ;;
    tests-asdlc) run_job tests-asdlc job_tests_asdlc ;;
    tdd) run_job tdd job_tdd ;;
    security)
      if [ "$DOCKER_TOOLS" -ne 1 ] && ! have gitleaks && ! have trivy; then
        job_security; skip security "gitleaks/trivy not installed; --docker-tools to use images"
      else run_job security job_security; fi ;;
    mutation|security-review) skip "$j" "CI-only (advisory/optional)" ;;
    *) echo "unknown job: $j" >&2; record "$j" "FAIL (unknown)" ;;
  esac
done

echo; echo "---- summary (base=$BASE) ----"
rc=0
for i in "${!names[@]}"; do
  printf '%-14s %s\n' "${names[$i]}" "${results[$i]}"
  [[ "${results[$i]}" == FAIL* ]] && rc=1
done
exit $rc
