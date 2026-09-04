#!/usr/bin/env bash
# Bootstrap the local deploy plane (EKS/ECR/ALB/ArgoCD stand-ins).
# Idempotent: safe to re-run. See deploy/README.md for the map to AWS.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
CLUSTER=marketplace
REG_NAME=kind-registry
REG_PORT=5001
INGRESS_VERSION=controller-v1.11.2

log() { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }

log "1/4 local registry (ECR stand-in) on localhost:${REG_PORT}"
if [ "$(docker inspect -f '{{.State.Running}}' ${REG_NAME} 2>/dev/null || true)" != "true" ]; then
  docker run -d --restart=always -p "127.0.0.1:${REG_PORT}:5000" --network bridge --name "${REG_NAME}" registry:2
fi

log "2/4 kind cluster (EKS stand-in): ${CLUSTER}"
if ! kind get clusters | grep -qx "${CLUSTER}"; then
  kind create cluster --name "${CLUSTER}" --config "${HERE}/kind-config.yaml"
fi
# Join the registry to the kind network so nodes can pull from it.
docker network connect kind "${REG_NAME}" 2>/dev/null || true
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:5001"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

log "3/4 ingress-nginx (ALB stand-in)"
kubectl apply -f "https://raw.githubusercontent.com/kubernetes/ingress-nginx/${INGRESS_VERSION}/deploy/static/provider/kind/deploy.yaml"
kubectl -n ingress-nginx wait --for=condition=available deploy/ingress-nginx-controller --timeout=240s || \
  kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=240s || true

log "4/4 ArgoCD (GitOps CD engine)"
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
# --server-side avoids the "metadata.annotations: Too long" error on ArgoCD's big CRDs.
kubectl apply --server-side -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl -n argocd rollout status deploy/argocd-server --timeout=360s || true

log "done. Next: deploy/gitea/bootstrap-gitea.sh then make -C deploy gitops-init"
