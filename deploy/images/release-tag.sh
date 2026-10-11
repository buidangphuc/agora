#!/usr/bin/env bash
# Release tagging (ASDLC W4-1): retag the local service images with the SHORT COMMIT SHA, push them
# to the local registry (ECR stand-in) and bump `images.<key>` in platform-gitops/envs/<ENV>/values.yaml.
# Idempotent: re-running with the same TAG re-pushes the same tag and rewrites identical values.
# It never commits anything; publish to ArgoCD with `make -C deploy gitops-push`.
#
#   make -C deploy release-tag [TAG=<sha>] [RELEASE_ENV=staging|local|prod] [DRY_RUN=1]
#
# TAG defaults to `git rev-parse --short=12 HEAD`. A dirty tree is refused (the tag would lie)
# (tracked or untracked) unless ALLOW_DIRTY=1. RELEASE_ENV=prod additionally needs CONFIRM_PROD=1.
# Services without a local image are skipped (same as push-images.sh).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REG="${REG:-localhost:5001}"
ENV_NAME="${RELEASE_ENV:-staging}"
DRY_RUN="${DRY_RUN:-0}"
GITOPS="${ROOT}/platform-gitops"
VALUES="${GITOPS}/envs/${ENV_NAME}/values.yaml"
SRC_PREFIXES="${SRC_PREFIXES:-agora- -}"
TAG="${TAG:-$(git -C "${ROOT}" rev-parse --short=12 HEAD)}"

case "${ENV_NAME}" in local|staging|prod) ;; *) echo "ENV must be local|staging|prod (got '${ENV_NAME}')" >&2; exit 2;; esac
if [ "${ENV_NAME}" = prod ] && [ "${CONFIRM_PROD:-0}" != 1 ]; then
  echo "RELEASE_ENV=prod rewrites the prod overlay: re-run with CONFIRM_PROD=1 after human approval." >&2
  exit 4
fi
[[ "${TAG}" =~ ^[0-9a-f]{7,40}$ ]] || { echo "TAG must be a hex commit sha (got '${TAG}')" >&2; exit 2; }
[ -f "${VALUES}" ] || { echo "missing ${VALUES}" >&2; exit 2; }
if [ "${ALLOW_DIRTY:-0}" != 1 ] && [ -n "$(git -C "${ROOT}" status --porcelain --untracked-files=all -- . ':!platform-gitops' ':!docs' ':!deploy' ':!scripts' ':!.claude' 2>/dev/null)" ]; then
  echo "working tree has uncommitted service changes: tag ${TAG} would not match the built code. Commit first or ALLOW_DIRTY=1." >&2
  exit 3
fi

MAP="$(grep -v '^[[:space:]]*#' "$(dirname "$0")/services.map")"

resolve_src() {
  local compose_image="$1" best="" best_created="" p cand created
  for p in ${SRC_PREFIXES}; do
    [ "${p}" = "-" ] && p=""
    cand="${p}${compose_image}"
    created="$(docker image inspect --format '{{.Created}}' "${cand}" 2>/dev/null)" || continue
    if [ -z "${best}" ] || [[ "${created}" > "${best_created}" ]]; then best="${cand}"; best_created="${created}"; fi
  done
  printf '%s' "${best}"
}

n=0
while read -r key repo compose_image; do
  [ -z "${key}" ] && continue
  # Non-local envs only carry the services they deploy: do not grow their images map.
  if [ "${ENV_NAME}" != local ] && [ "$(key="${key}" yq '.images | has(env(key))' "${VALUES}")" != true ]; then continue; fi
  if [ "${DRY_RUN}" = 1 ]; then
    echo "[dry-run] ${compose_image} -> ${REG}/${repo}:${TAG}; ${VALUES#${ROOT}/}: images.${key} = ${TAG}"
    n=$((n+1)); continue
  fi
  src="$(resolve_src "${compose_image}")"
  if [ -z "${src}" ]; then echo "skip ${key} (no local image ${compose_image})"; continue; fi
  docker tag "${src}" "${REG}/${repo}:${TAG}"
  docker push "${REG}/${repo}:${TAG}" >/dev/null && echo "pushed ${REG}/${repo}:${TAG} (from ${src})"
  key="${key}" tag="${TAG}" yq -i '.images[env(key)] = env(tag)' "${VALUES}"
  if [ "${ENV_NAME}" = local ]; then
    for f in "${GITOPS}"/platform/*/kustomization.yaml; do
      [ -f "${f}" ] || continue
      name="${REG}/${repo}" tag="${TAG}" yq -i '(.images[] | select(.name == env(name))).newTag = env(tag)' "${f}"
    done
  fi
  n=$((n+1))
done <<EOF2
${MAP}
EOF2
echo "release-tag: ${n} image(s) -> tag ${TAG} in envs/${ENV_NAME}/values.yaml$([ "${DRY_RUN}" = 1 ] && echo ' (dry-run, nothing changed)'). Publish: make -C deploy gitops-push"
