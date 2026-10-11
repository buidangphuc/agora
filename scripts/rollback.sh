#!/usr/bin/env bash
# One-line rollback for a canary-managed service (ASDLC W4-3). See docs/runbooks/rollback.md.
#
#   scripts/rollback.sh <service> [--env staging|local] [--to <revision|tag>] [--abort] [--dry-run]
#
#   (no flags)        kubectl argo rollouts undo  -> previous revision, immediate (cluster only)
#   --abort           kubectl argo rollouts abort -> stop an in-flight canary, traffic back to stable
#   --to <number>     undo --to-revision <number>
#   --to <tag>        DURABLE: set images.<service> = <tag> in platform-gitops/envs/<env>/values.yaml
#                     (publish with `make -C deploy gitops-push`; ArgoCD then rolls the Rollout back)
# Why --to <tag> exists: ArgoCD selfHeal re-applies git, so a bare `undo` is rolled forward again
# unless git is reverted too. `abort` is not reverted (it lives in status, not spec).
# Needs: kubectl + the argo-rollouts plugin (brew install argoproj/tap/kubectl-argo-rollouts); yq for --to <tag>.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
usage() { sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

SVC="${1:-}"; [ -n "${SVC}" ] && [[ "${SVC}" != -* ]] || usage
shift
ENV_NAME=staging; TO=""; ABORT=0; DRY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --env) ENV_NAME="${2:?--env needs a value}"; shift 2;;
    --to) TO="${2:?--to needs a value}"; shift 2;;
    --abort) ABORT=1; shift;;
    --dry-run) DRY=1; shift;;
    -h|--help) usage;;
    *) echo "unknown arg: $1" >&2; usage;;
  esac
done
case "${ENV_NAME}" in
  staging) NS=marketplace-staging;;
  local) NS=marketplace;;
  *) echo "--env must be staging|local (prod has no cluster here)" >&2; exit 2;;
esac
run() { echo "+ $*"; [ "${DRY}" = 1 ] || "$@"; }

if [ -n "${TO}" ] && ! [[ "${TO}" =~ ^[0-9]+$ ]]; then
  [[ "${TO}" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "bad tag '${TO}'" >&2; exit 2; }
  VALUES="${ROOT}/platform-gitops/envs/${ENV_NAME}/values.yaml"
  echo "+ yq -i '.images[\"${SVC}\"] = \"${TO}\"' ${VALUES#${ROOT}/}"
  [ "${DRY}" = 1 ] || svc="${SVC}" tag="${TO}" yq -i '.images[env(svc)] = env(tag)' "${VALUES}"
  echo "next: make -C deploy gitops-push   # ArgoCD rolls ${SVC} back to tag ${TO}"
  exit 0
fi

command -v kubectl >/dev/null || { echo "kubectl not found" >&2; exit 1; }
if [ "${DRY}" != 1 ] && ! kubectl argo rollouts version >/dev/null 2>&1; then
  echo "kubectl argo-rollouts plugin missing: brew install argoproj/tap/kubectl-argo-rollouts" >&2; exit 1
fi
if [ "${ABORT}" = 1 ]; then
  run kubectl argo rollouts abort "${SVC}" -n "${NS}"
elif [ -n "${TO}" ]; then
  run kubectl argo rollouts undo "${SVC}" -n "${NS}" --to-revision "${TO}"
else
  run kubectl argo rollouts undo "${SVC}" -n "${NS}"
fi
[ "${ABORT}" = 1 ] || echo "note: ArgoCD selfHeal re-applies git; make it durable with: scripts/rollback.sh ${SVC} --env ${ENV_NAME} --to <good-tag> && make -C deploy gitops-push"
[ "${DRY}" = 1 ] || kubectl argo rollouts get rollout "${SVC}" -n "${NS}" | head -20
