#!/usr/bin/env bash
# Task 2.3 - capability "Order lifecycle contract". Scenarios:
#   "Order lifecycle contract: Copies match the source"
#   "Order lifecycle contract: Drifted or missing copy is rejected"
#   "Order lifecycle contract: Consumer copies lint"
# Usage: [REPO_ROOT=...] bash platform-core/packages/proto/consumer_copies_test.sh
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"
ORDER="platform/order/v1/order.proto"
PAY="platform/payment/v1/payment.proto"
REPOS="team-frontend team-order team-engagement team-gateway team-domain team-notification team-chat team-payment team-search team-analytics"

# check_copies <root>: prints FAIL lines, returns non-zero on any drift/missing (cmp fails on missing).
check_copies() {
  local root="$1" rc=0 r
  for r in $REPOS; do
    cmp -s "$root/$SRC_REL/order/v1/order.proto" "$root/$r/proto/$ORDER" \
      || { echo "FAIL copies-match: $r/proto/$ORDER"; rc=1; }
    cmp -s "$root/$SRC_REL/payment/v1/payment.proto" "$root/$r/proto/$PAY" \
      || { echo "FAIL copies-match: $r/proto/$PAY"; rc=1; }
    grep -q ConfirmReceipt "$root/$r/proto/$ORDER" 2>/dev/null \
      || { echo "FAIL content: $r order.proto lacks ConfirmReceipt"; rc=1; }
    grep -q PAYMENT_STATUS_CANCELLED "$root/$r/proto/$PAY" 2>/dev/null \
      || { echo "FAIL content: $r payment.proto lacks PAYMENT_STATUS_CANCELLED"; rc=1; }
  done
  return $rc
}

fail=0

# Scenario: Copies match the source
if out="$(check_copies "$ROOT")"; then echo "ok   copies match (20)"; else echo "$out"; fail=1; fi

# Scenario: Drifted or missing copy is rejected (the check itself, on a synthetic tree)
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
mk() { # build a faithful synthetic tree
  rm -rf "$tmp/t"; mkdir -p "$tmp/t/$SRC_REL/order/v1" "$tmp/t/$SRC_REL/payment/v1"
  printf 'ConfirmReceipt\n' > "$tmp/t/$SRC_REL/order/v1/order.proto"
  printf 'PAYMENT_STATUS_CANCELLED\n' > "$tmp/t/$SRC_REL/payment/v1/payment.proto"
  for r in $REPOS; do
    mkdir -p "$tmp/t/$r/proto/platform/order/v1" "$tmp/t/$r/proto/platform/payment/v1"
    cp "$tmp/t/$SRC_REL/order/v1/order.proto" "$tmp/t/$r/proto/$ORDER"
    cp "$tmp/t/$SRC_REL/payment/v1/payment.proto" "$tmp/t/$r/proto/$PAY"
  done
}
mk
check_copies "$tmp/t" >/dev/null && echo "ok   faithful tree accepted" || { echo "FAIL faithful synthetic tree rejected"; fail=1; }
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"
check_copies "$tmp/t" >/dev/null && { echo "FAIL one-byte drift accepted"; fail=1; } || echo "ok   one-byte drift rejected"
mk; rm "$tmp/t/team-chat/proto/$PAY"
check_copies "$tmp/t" >/dev/null && { echo "FAIL missing copy accepted"; fail=1; } || echo "ok   missing copy rejected"

# Scenario: Consumer copies lint
for r in $REPOS; do
  if (cd "$ROOT/$r/proto" && buf lint >/dev/null 2>"$tmp/lint.err"); then echo "ok   buf lint $r"
  else echo "FAIL buf lint $r: $(head -3 "$tmp/lint.err" | tr '\n' ' ')"; fail=1; fi
done
exit $fail
