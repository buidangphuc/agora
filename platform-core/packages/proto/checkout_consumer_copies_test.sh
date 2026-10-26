#!/usr/bin/env bash
# Task 2.3 - capability "Checkout contract". Scenarios:
#   "Checkout contract: Copies match the source"
#   "Checkout contract: Drifted or missing copy is rejected"
#   "Checkout contract: Consumer copies build and lint"
# Usage: [REPO_ROOT=...] bash platform-core/packages/proto/checkout_consumer_copies_test.sh
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"
ORDER="platform/order/v1/order.proto"
PROMO="platform/promotion/v1/promotion.proto"
ORDER_REPOS="team-analytics team-chat team-domain team-engagement team-frontend team-gateway team-notification team-order team-payment team-search"
PROMO_REPOS="team-analytics team-frontend team-gateway team-order team-promotion"
BUF="${BUF:-buf}"

check_copies() {
  local root="$1" rc=0 r
  for r in $ORDER_REPOS; do
    cmp -s "$root/$SRC_REL/order/v1/order.proto" "$root/$r/proto/$ORDER" \
      || { echo "FAIL copies-match: $r/proto/$ORDER"; rc=1; }
  done
  for r in $PROMO_REPOS; do
    cmp -s "$root/$SRC_REL/promotion/v1/promotion.proto" "$root/$r/proto/$PROMO" \
      || { echo "FAIL copies-match: $r/proto/$PROMO"; rc=1; }
  done
  return $rc
}

check_no_new_files() {
  local root="$1" rc=0 d r
  for d in "$root"/team-*/proto; do
    r="$(basename "$(dirname "$d")")"
    if [ -f "$d/$ORDER" ] && ! echo " $ORDER_REPOS " | grep -q " $r "; then echo "FAIL new file: $r/proto/$ORDER"; rc=1; fi
    if [ -f "$d/$PROMO" ] && ! echo " $PROMO_REPOS " | grep -q " $r "; then echo "FAIL new file: $r/proto/$PROMO"; rc=1; fi
  done
  return $rc
}

fail=0
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# Scenario: Copies match the source
if out="$(check_copies "$ROOT")"; then echo "ok   copies match (15)"; else echo "$out"; fail=1; fi
for r in $ORDER_REPOS; do
  grep -q PreviewCheckout "$ROOT/$r/proto/$ORDER" 2>/dev/null \
    || { echo "FAIL content: $r order.proto lacks PreviewCheckout"; fail=1; }
done
if out="$(check_no_new_files "$ROOT")"; then echo "ok   no new files"; else echo "$out"; fail=1; fi

# Scenario: Drifted or missing copy is rejected (the check itself, on a synthetic tree)
mk() {
  rm -rf "$tmp/t"; mkdir -p "$tmp/t/$SRC_REL/order/v1" "$tmp/t/$SRC_REL/promotion/v1"
  printf 'PreviewCheckout\n' > "$tmp/t/$SRC_REL/order/v1/order.proto"
  printf 'promo\n' > "$tmp/t/$SRC_REL/promotion/v1/promotion.proto"
  for r in $ORDER_REPOS; do
    mkdir -p "$tmp/t/$r/proto/platform/order/v1"; cp "$tmp/t/$SRC_REL/order/v1/order.proto" "$tmp/t/$r/proto/$ORDER"
  done
  for r in $PROMO_REPOS; do
    mkdir -p "$tmp/t/$r/proto/platform/promotion/v1"; cp "$tmp/t/$SRC_REL/promotion/v1/promotion.proto" "$tmp/t/$r/proto/$PROMO"
  done
}
mk
check_copies "$tmp/t" >/dev/null && echo "ok   faithful tree accepted" || { echo "FAIL faithful synthetic tree rejected"; fail=1; }
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"
check_copies "$tmp/t" >/dev/null && { echo "FAIL one-byte order drift accepted"; fail=1; } || echo "ok   one-byte order drift rejected"
mk; printf 'x' >> "$tmp/t/team-gateway/proto/$PROMO"
check_copies "$tmp/t" >/dev/null && { echo "FAIL one-byte promotion drift accepted"; fail=1; } || echo "ok   one-byte promotion drift rejected"
mk; rm "$tmp/t/team-chat/proto/$ORDER"
check_copies "$tmp/t" >/dev/null && { echo "FAIL missing order copy accepted"; fail=1; } || echo "ok   missing order copy rejected"
mk; rm "$tmp/t/team-promotion/proto/$PROMO"
check_copies "$tmp/t" >/dev/null && { echo "FAIL missing promotion copy accepted"; fail=1; } || echo "ok   missing promotion copy rejected"

# Scenario: Consumer copies build and lint
for r in $ORDER_REPOS $PROMO_REPOS; do
  case " $ORDER_REPOS " in *" $r "*) paths="--path $ORDER";; *) paths="";; esac
  case " $PROMO_REPOS " in *" $r "*) paths="$paths --path $PROMO";; esac
  if (cd "$ROOT/$r/proto" && $BUF lint $paths >"$tmp/e.err" 2>&1 && $BUF build $paths >>"$tmp/e.err" 2>&1); then echo "ok   buf lint+build $r"
  else echo "FAIL buf lint/build $r: $(head -3 "$tmp/e.err" | tr '\n' ' ')"; fail=1; fi
done
exit $fail
