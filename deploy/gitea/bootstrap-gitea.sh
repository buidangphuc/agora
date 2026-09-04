#!/usr/bin/env bash
# Deploy Gitea, create the platform-gitops repo, push it, register it in ArgoCD,
# and apply the app-of-apps. Idempotent-ish. Run after deploy/kind/bootstrap.sh.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "${HERE}/../.." && pwd)"
GITOPS_DIR="${ROOT}/platform-gitops"
GITEA_URL="http://gitea.localtest.me"
GIT_USER="ci"
GIT_PASS="ci-pass-123"
REPO="platform-gitops"

log() { printf '\n\033[1;35m== %s\033[0m\n' "$*"; }

log "1/6 deploy Gitea"
kubectl apply -f "${HERE}/gitea.yaml"
kubectl -n gitea rollout status deploy/gitea --timeout=240s

log "2/6 wait for ingress route ${GITEA_URL}"
for i in $(seq 1 60); do
  curl -sf -o /dev/null "${GITEA_URL}/api/healthz" && break || sleep 3
done

log "3/6 admin user + token + repo"
kubectl -n gitea exec deploy/gitea -- su-exec git gitea admin user create \
  --admin --username "${GIT_USER}" --password "${GIT_PASS}" \
  --email ci@local --must-change-password=false 2>/dev/null || echo "  (user exists)"
# fresh token each run (name must be unique) — delete old, create new
curl -s -u "${GIT_USER}:${GIT_PASS}" -X DELETE "${GITEA_URL}/api/v1/users/${GIT_USER}/tokens/argocd" >/dev/null 2>&1 || true
TOKEN=$(curl -s -u "${GIT_USER}:${GIT_PASS}" -X POST "${GITEA_URL}/api/v1/users/${GIT_USER}/tokens" \
  -H 'Content-Type: application/json' -d '{"name":"argocd","scopes":["all"]}' | yq -r '.sha1')
curl -s -H "Authorization: token ${TOKEN}" -X POST "${GITEA_URL}/api/v1/user/repos" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"${REPO}\",\"private\":false,\"default_branch\":\"main\",\"auto_init\":false}" >/dev/null || true

log "4/6 push platform-gitops"
cd "${GITOPS_DIR}"
git init -q -b main 2>/dev/null || true
git add -A
git -c user.email=ci@local -c user.name=ci commit -q -m "gitops: sync $(git rev-parse --short HEAD 2>/dev/null || echo init)" || echo "  (nothing to commit)"
git remote remove origin 2>/dev/null || true
git remote add origin "http://${GIT_USER}:${GIT_PASS}@gitea.localtest.me/${GIT_USER}/${REPO}.git"
git push -q -f -u origin main

log "5/6 register repo creds in ArgoCD"
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: gitea-${REPO}
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: git
  url: http://gitea-http.gitea.svc.cluster.local:3000/${GIT_USER}/${REPO}.git
  username: ${GIT_USER}
  password: ${GIT_PASS}
EOF

log "6/6 apply app-of-apps"
kubectl apply -f "${ROOT}/deploy/argocd/root-app.yaml"
echo "done. ArgoCD admin password:"
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d; echo
