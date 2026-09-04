#!/usr/bin/env bash
# Configure Vault (dev) for per-service isolation: k8s auth, kv engine, shared/jwt,
# and a policy+role+seed per service. Idempotent (dev re-seed is safe).
# Usage: bash deploy/vault/bootstrap-vault.sh [service1 service2 ...]
set -euo pipefail
NS=vault
POD=vault-0
SERVICES=("$@")
[ ${#SERVICES[@]} -eq 0 ] && SERVICES=(team-gateway team-identity)

# Pipe a fully-interpolated script into the vault pod (root token, dev).
vrun() { kubectl -n "$NS" exec -i "$POD" -- sh -c 'VAULT_ADDR=http://127.0.0.1:8200 VAULT_TOKEN=root sh -s'; }

echo "== vault SA -> system:auth-delegator (review k8s tokens) =="
kubectl create clusterrolebinding vault-auth-delegator \
  --clusterrole=system:auth-delegator \
  --serviceaccount="${NS}:vault" --dry-run=client -o yaml | kubectl apply -f -

echo "== k8s auth + kv engine + shared/jwt =="
cat <<'EOF' | vrun
set -e
vault auth enable kubernetes 2>/dev/null || true
vault write auth/kubernetes/config kubernetes_host="https://kubernetes.default.svc" >/dev/null
vault secrets enable -path=svc kv-v2 2>/dev/null || true
vault kv put svc/shared/jwt JWT_SECRET="dev-secret-change-me" >/dev/null
echo "  shared/jwt seeded"
EOF

for svc in "${SERVICES[@]}"; do
  u="${svc//-/_}"
  echo "== per-service isolation: ${svc} =="
  cat <<EOF | vrun
set -e
vault kv put svc/${svc}/config DATABASE_URL="postgres://${u}_svc:${u}_pass@postgres.marketplace.svc:5432/${u}_db?sslmode=disable" HTTP_PORT="8080" >/dev/null
printf 'path "svc/data/${svc}/*" { capabilities = ["read"] }\npath "svc/data/shared/jwt" { capabilities = ["read"] }\n' > /tmp/${svc}.hcl
vault policy write ${svc} /tmp/${svc}.hcl >/dev/null
vault write auth/kubernetes/role/${svc} bound_service_account_names=${svc} bound_service_account_namespaces=marketplace policies=${svc} ttl=1h >/dev/null
echo "  ${svc}: kv + policy + role done"
EOF
done
echo "done."
