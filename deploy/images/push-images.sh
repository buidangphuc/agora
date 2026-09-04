#!/usr/bin/env bash
# Retag local service images to the local registry (ECR stand-in), push, and write
# the tag into platform-gitops/envs/local/values.yaml (CI does this in real life).
# Portable to macOS bash 3.2 (no associative arrays).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REG=localhost:5001
VALUES="${ROOT}/platform-gitops/envs/local/values.yaml"
TAG="${TAG:-local}"

# "<service> <source-image-on-host>" — source images are the :local builds.
MAP="
team-gateway team-gateway:local
team-identity team-identity:local
team-domain team-domain:local
team-search team-search:local
team-engagement team-engagement:local
team-order team-order:local
team-payment team-payment:local
team-chat team-chat:local
team-notification team-notification:local
team-ai ai-platform:local
"

while read -r svc src; do
  [ -z "${svc}" ] && continue
  if docker image inspect "${src}" >/dev/null 2>&1; then
    docker tag "${src}" "${REG}/${svc}:${TAG}"
    docker push "${REG}/${svc}:${TAG}" >/dev/null && echo "pushed ${REG}/${svc}:${TAG}"
    svc="${svc}" tag="${TAG}" yq -i '.images[env(svc)] = env(tag)' "${VALUES}"
  else
    echo "skip ${svc} (no local image ${src})"
  fi
done <<EOF
${MAP}
EOF
echo "updated ${VALUES}"
