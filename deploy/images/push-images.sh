#!/usr/bin/env bash
# Retag local service images to the local registry (ECR stand-in), push, and write
# the tag into platform-gitops (CI does this in real life):
#   - envs/local/values.yaml            images.<key>           (Helm charts/service apps)
#   - platform/*/kustomization.yaml     images[].newTag        (raw-manifest apps)
# Portable to macOS bash 3.2 (no associative arrays).
#
# The tag defaults to a content-derived one (sha-<12 hex of the image id>), so a rebuilt image
# always changes the manifest and ArgoCD rolls it; a fixed `local` tag + IfNotPresent would keep
# serving the node's stale cache. `TAG=local make -C deploy images` forces a fixed tag.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REG=localhost:5001
GITOPS="${ROOT}/platform-gitops"
VALUES="${GITOPS}/envs/local/values.yaml"
TAG="${TAG:-}"
# The compose project may be run under an isolation override that renames images to
# agora-<name>:local (platform-e2e/compose). Both spellings are candidates; the newest build wins.
# Space-separated; "-" stands for no prefix (an empty word would be dropped by word splitting).
SRC_PREFIXES="${SRC_PREFIXES:-agora- -}"

# services.map rows: "<values-key> <registry-repo> <compose-image>" — compose-image is the `image:` the compose files
# declare (docker-compose.services.yaml); the registry repo is what the gitops manifests pull.
# search-indexer is the same image as team-search (entrypoint /indexer, set in the manifest).
MAP="$(grep -v '^[[:space:]]*#' "$(dirname "$0")/services.map")"

# Newest existing local image among the candidate spellings of a compose image name.
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

set_kustomize_tag() { # <registry-repo> <tag>
  local f
  for f in "${GITOPS}"/platform/*/kustomization.yaml; do
    [ -f "${f}" ] || continue
    name="${REG}/$1" tag="$2" yq -i '(.images[] | select(.name == env(name))).newTag = env(tag)' "${f}"
  done
}

while read -r key repo compose_image; do
  [ -z "${key}" ] && continue
  src="$(resolve_src "${compose_image}")"
  if [ -z "${src}" ]; then
    echo "skip ${key} (no local image ${compose_image}, tried prefixes: '${SRC_PREFIXES}')"
    continue
  fi
  tag="${TAG:-sha-$(docker image inspect --format '{{.Id}}' "${src}" | sed 's/^sha256://' | cut -c1-12)}"
  docker tag "${src}" "${REG}/${repo}:${tag}"
  docker push "${REG}/${repo}:${tag}" >/dev/null && echo "pushed ${REG}/${repo}:${tag} (from ${src})"
  key="${key}" tag="${tag}" yq -i '.images[env(key)] = env(tag)' "${VALUES}"
  set_kustomize_tag "${repo}" "${tag}"
done <<EOF2
${MAP}
EOF2
echo "updated ${VALUES} and platform/*/kustomization.yaml; publish with: make -C deploy gitops-push"
