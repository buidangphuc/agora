#!/usr/bin/env bash
# Change q7-referral-welcome-contract, task 3.1. Scenarios:
#   "Q7 referral welcome contract: Copies match the source"
#   "Q7 referral welcome contract: Drifted or missing copy is rejected"
#   "Q7 referral welcome contract: Referral has one canonical source"
#   "Q7 referral welcome contract: Consumer protos build"
# Usage: [REPO_ROOT=...] bash platform-core/packages/proto/q7_consumer_copies_test.sh
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"

IDENTITY="platform/identity/v1/identity.proto"
ORDER="platform/order/v1/order.proto"
PROMOTION="platform/promotion/v1/promotion.proto"
REFERRAL="platform/referral/v1/referral.proto"
PAYMENT="platform/payment/v1/payment.proto"
EVENTS="platform/events/v1/events.proto"

IDENTITY_REPOS="team-analytics team-chat team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-identity team-notification team-order team-payment team-search team-referral team-promotion"
ORDER_REPOS="team-analytics team-chat team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-notification team-order team-payment team-search team-referral team-promotion"
PROMOTION_REPOS="team-analytics team-frontend team-frontend-admin team-frontend-seller team-gateway team-order team-promotion"
REFERRAL_REPOS="team-analytics team-frontend team-frontend-admin team-frontend-seller team-gateway team-referral team-promotion"
PAYMENT_REPOS="team-referral team-promotion"
EVENTS_REPOS="team-referral"

pairs() {
  local r
  for r in $IDENTITY_REPOS; do echo "$IDENTITY $r"; done
  for r in $ORDER_REPOS; do echo "$ORDER $r"; done
  for r in $PROMOTION_REPOS; do echo "$PROMOTION $r"; done
  for r in $REFERRAL_REPOS; do echo "$REFERRAL $r"; done
  for r in $PAYMENT_REPOS; do echo "$PAYMENT $r"; done
  for r in $EVENTS_REPOS; do echo "$EVENTS $r"; done
}
NPAIRS="$(pairs | wc -l | tr -d ' ')"

check_copies() {
  local root="$1" rc=0 rel r
  while read -r rel r; do
    cmp -s "$root/$SRC_REL/${rel#platform/}" "$root/$r/proto/$rel" \
      || { echo "FAIL copies-match: $r/proto/$rel"; rc=1; }
  done < <(pairs)
  return $rc
}

fail=0
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# Scenario: Copies match the source
if out="$(check_copies "$ROOT")"; then echo "ok   copies match ($NPAIRS)"; else echo "$out"; fail=1; fi

# Scenario: Drifted or missing copy is rejected (the check itself, on a synthetic tree)
mk() {
  rm -rf "$tmp/t"
  local rel r
  while read -r rel r; do
    mkdir -p "$tmp/t/$SRC_REL/$(dirname "${rel#platform/}")" "$tmp/t/$r/proto/$(dirname "$rel")"
    [ -f "$tmp/t/$SRC_REL/${rel#platform/}" ] || printf 'source %s\n' "$rel" > "$tmp/t/$SRC_REL/${rel#platform/}"
    cp "$tmp/t/$SRC_REL/${rel#platform/}" "$tmp/t/$r/proto/$rel"
  done < <(pairs)
}
mk
check_copies "$tmp/t" >/dev/null && echo "ok   faithful tree accepted" || { echo "FAIL faithful synthetic tree rejected"; fail=1; }
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"; rm "$tmp/t/team-referral/proto/$IDENTITY"
if out="$(check_copies "$tmp/t")"; then echo "FAIL drift and missing copy accepted"; fail=1
elif echo "$out" | grep -q "team-order/proto/$ORDER" && echo "$out" | grep -q "team-referral/proto/$IDENTITY"; then echo "ok   drifted and missing copy rejected and named"
else echo "FAIL drift/missing rejected without naming both: $out"; fail=1; fi

# Scenario: Referral has one canonical source
if cmp -s "$ROOT/$SRC_REL/referral/v1/referral.proto" "$ROOT/team-referral/proto/$REFERRAL"; then echo "ok   referral copy equals canonical source"
else echo "FAIL referral canonical: team-referral/proto/$REFERRAL differs from $SRC_REL/referral/v1/referral.proto"; fail=1; fi

# Scenario: Consumer protos build
NEED_team_referral="$IDENTITY $ORDER $PAYMENT $EVENTS $REFERRAL"
NEED_team_promotion="$IDENTITY $REFERRAL $ORDER $PAYMENT $PROMOTION"
for r in team-referral team-promotion; do
  need="NEED_${r//-/_}"
  for rel in ${!need}; do
    [ -f "$ROOT/$r/proto/$rel" ] || { echo "FAIL build-inputs: $r/proto/$rel missing"; fail=1; }
  done
  if (cd "$ROOT/$r/proto" && buf build >"$tmp/build.err" 2>&1); then echo "ok   buf build $r"
  else echo "FAIL buf build $r: $(head -3 "$tmp/build.err" | tr '\n' ' ')"; fail=1; fi
done
exit $fail
