#!/usr/bin/env bash
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"

IDENTITY="platform/identity/v1/identity.proto"
ORDER="platform/order/v1/order.proto"
PAYMENT="platform/payment/v1/payment.proto"
VERIFICATION="platform/verification/v1/verification.proto"
AUDIT="platform/audit/v1/audit.proto"
EVENTS="platform/events/v1/events.proto"

IPO_REPOS="team-analytics team-chat team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-notification team-order team-payment team-promotion team-referral team-search"
IDENTITY_REPOS="$IPO_REPOS team-identity team-verification"
VERIFICATION_REPOS="team-analytics team-frontend team-frontend-admin team-frontend-seller team-gateway team-verification"
AUDIT_REPOS="team-analytics team-audit team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-identity team-order team-payment team-promotion team-verification"
EVENTS_REPOS="team-verification"

pairs() {
  local r
  for r in $IDENTITY_REPOS; do echo "$IDENTITY $r"; done
  for r in $IPO_REPOS; do echo "$ORDER $r"; done
  for r in $IPO_REPOS; do echo "$PAYMENT $r"; done
  for r in $VERIFICATION_REPOS; do echo "$VERIFICATION $r"; done
  for r in $AUDIT_REPOS; do echo "$AUDIT $r"; done
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

if out="$(check_copies "$ROOT")"; then echo "ok   copies match ($NPAIRS)"; else echo "$out"; fail=1; fi

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
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"; rm "$tmp/t/team-verification/proto/$EVENTS"
if out="$(check_copies "$tmp/t")"; then echo "FAIL drift and missing copy accepted"; fail=1
elif echo "$out" | grep -q "team-order/proto/$ORDER" && echo "$out" | grep -q "team-verification/proto/$EVENTS"; then echo "ok   drifted and missing copy rejected and named"
else echo "FAIL drift/missing rejected without naming both: $out"; fail=1; fi

for r in $(echo "$IDENTITY_REPOS $VERIFICATION_REPOS $AUDIT_REPOS $EVENTS_REPOS" | tr ' ' '\n' | sort -u); do
  [ -f "$ROOT/$r/proto/buf.yaml" ] || continue
  if (cd "$ROOT/$r/proto" && buf build >"$tmp/build.err" 2>&1); then echo "ok   buf build $r"
  else echo "FAIL buf build $r: $(head -3 "$tmp/build.err" | tr '\n' ' ')"; fail=1; fi
done
exit $fail
