#!/usr/bin/env bash
# task 11 (order-receipt-completion) + task 1.8 (order-go-sdk-v1): README and coverage.summary document the migrated service.
set -u
d="$(dirname "$0")/.."
f="$d/README.md"; cov="$d/coverage.summary"; rc=0
need() { grep -qF -- "$1" "$f" || { echo "FAIL: README missing: $1"; rc=1; }; }
gone() { grep -qF -- "$1" "$f" && { echo "FAIL: README still mentions: $1"; rc=1; }; return 0; }
for k in AUTO_COMPLETE_AFTER OrderCompleted; do need "$k"; done
for k in "migrate up" "liveness" "0011" "0022" "scale the old pods to 0" "PreSync" "7 days" \
  "audit trail unavailable" "internal/rpc" "internal/order" "internal/store" "internal/events" \
  "GRPC_REFLECTION_ENABLED" "UPSTREAM_PROMOTION_ADDR" "UPSTREAM_AUDIT_ADDR" "resync-commits"; do need "$k"; done
for k in DATABASE_ENABLED KAFKA_ENABLED OUTBOX_ENABLED FEATURE_FLAGS_ENABLED "in-memory" "internal/handler" \
  "internal/service" "internal/repository" "internal/consumer" "internal/bootstrap" "internal/grpcserver" "internal/interceptor"; do gone "$k"; done
grep -qi "ignored without .UPSTREAM_PROMOTION_ADDR\|no vouchers" "$f" && { echo "FAIL: README keeps promotion-less voucher mode"; rc=1; }
grep -q "GRPC_REFLECTION_ENABLED. | .false" "$f" || { echo "FAIL: README must show reflection default false"; rc=1; }
[ -s "$cov" ] || { echo "FAIL: coverage.summary empty"; rc=1; }
for k in internal/handler internal/service/ internal/repository internal/consumer internal/bootstrap internal/grpcserver internal/interceptor; do
  grep -q "$k" "$cov" && { echo "FAIL: coverage.summary has stale path $k"; rc=1; }
done
grep -q "internal/rpc/" "$cov" || { echo "FAIL: coverage.summary lacks internal/rpc"; rc=1; }
grep -q "internal/store/" "$cov" || { echo "FAIL: coverage.summary lacks internal/store"; rc=1; }
exit $rc
