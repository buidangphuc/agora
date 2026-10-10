#!/usr/bin/env bash
# Publish the platform-gitops tree to the LOCAL Gitea repo that ArgoCD watches.
#
# platform-gitops/ is a directory of the agora monorepo (origin = GitHub), not a repo of its own.
# So this never runs git add/commit/push inside the agora checkout: it clones the Gitea repo into
# a scratch dir, mirrors the working tree over it, commits there and pushes to Gitea only.
# The scratch dir (and the credentialed URL) is removed on exit.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${ROOT}/platform-gitops"
GITEA_HOST="${GITEA_HOST:-gitea.localtest.me}"
GIT_USER="${GIT_USER:-ci}"
GIT_PASS="${GIT_PASS:-ci-pass-123}" # local-only Gitea admin (see bootstrap-gitea.sh); no real secret
REPO="${GITEA_REPO:-platform-gitops}"
REMOTE="http://${GIT_USER}:${GIT_PASS}@${GITEA_HOST}/${GIT_USER}/${REPO}.git"

case "${GITEA_HOST}" in
  gitea.localtest.me | localhost | localhost:* | 127.0.0.1 | 127.0.0.1:*) ;;
  *) echo "refusing to publish: GITEA_HOST=${GITEA_HOST} is not a local Gitea" >&2; exit 1 ;;
esac

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
ID=(-c user.email=ci@local -c user.name=ci)

if git clone -q "${REMOTE}" "${WORK}/repo" 2>/dev/null; then :; else
  mkdir "${WORK}/repo" && git -C "${WORK}/repo" init -q -b main
  git -C "${WORK}/repo" remote add gitea "${REMOTE}"
fi
cd "${WORK}/repo"
git checkout -q -B main 2>/dev/null || true
rsync -a --delete --exclude '.git' --exclude '.DS_Store' "${SRC}/" "${WORK}/repo/"
git add -A
if git diff --cached --quiet; then
  echo "gitea ${REPO}: nothing to publish"
else
  git "${ID[@]}" commit -q -m "gitops: publish $(git -C "${ROOT}" rev-parse --short HEAD 2>/dev/null || echo local)"
fi
REMOTE_NAME="$(git remote | head -n1)"
git push -q "${REMOTE_NAME}" HEAD:main
echo "gitea ${REPO}: main at $(git rev-parse --short HEAD)"
