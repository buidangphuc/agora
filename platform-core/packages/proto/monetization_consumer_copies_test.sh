#!/usr/bin/env bash
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"
SRC_MOD="$ROOT/platform-core/packages/proto"
BASE_REF="${BASE_REF:-origin/main}"

ORDER="platform/order/v1/order.proto"
PAYMENT="platform/payment/v1/payment.proto"
PROMOTION="platform/promotion/v1/promotion.proto"

ORDER_REPOS="team-analytics team-chat team-domain team-engagement team-frontend team-frontend-admin team-frontend-seller team-gateway team-notification team-order team-payment team-promotion team-referral team-search"
PAYMENT_REPOS="$ORDER_REPOS"
PROMOTION_REPOS="team-analytics team-frontend team-frontend-admin team-frontend-seller team-gateway team-order team-promotion"

pairs() {
  local r
  for r in $ORDER_REPOS; do echo "$ORDER $r"; done
  for r in $PAYMENT_REPOS; do echo "$PAYMENT $r"; done
  for r in $PROMOTION_REPOS; do echo "$PROMOTION $r"; done
}
NPAIRS="$(pairs | wc -l | tr -d ' ')"
UNION="$(pairs | awk '{print $2}' | sort -u)"

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

if [ "$NPAIRS" != 35 ]; then echo "FAIL pair table has $NPAIRS entries, expected 35 (14+14+7)"; fail=1; fi

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
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"; rm "$tmp/t/team-payment/proto/$PAYMENT"
if out="$(check_copies "$tmp/t")"; then echo "FAIL drift and missing copy accepted"; fail=1
elif echo "$out" | grep -q "team-order/proto/$ORDER" && echo "$out" | grep -q "team-payment/proto/$PAYMENT"; then echo "ok   drifted and missing copy rejected and named"
else echo "FAIL drift/missing rejected without naming both: $out"; fail=1; fi

renum() {
  local label="$1" rel="$2" expr="$3" name="$4"
  rm -rf "$tmp/mod"; mkdir -p "$tmp/mod"; cp -R "$SRC_MOD/buf.yaml" "$SRC_MOD/platform" "$tmp/mod/"
  if sed -i.bak -E "$expr" "$tmp/mod/$rel" && ! cmp -s "$tmp/mod/$rel" "$SRC_MOD/$rel"; then
    if (cd "$tmp/mod" && buf breaking --against "$SRC_MOD") >"$tmp/renum.out" 2>&1; then echo "FAIL renumbering $label accepted by buf breaking"; fail=1
    elif grep -q "$name" "$tmp/renum.out"; then echo "ok   renumbering $label rejected and names $name"
    else echo "FAIL renumbering $label rejected without naming $name: $(head -3 "$tmp/renum.out" | tr '\n' ' ')"; fail=1; fi
  else echo "FAIL renumbering $label: could not mutate scratch copy (element missing from source)"; fail=1; fi
}
renum "items_subtotal" "$ORDER" '/^message OrderPaidEvent /,/^}/ s/(items_subtotal = )9;/\119;/' "items_subtotal"
renum "COMMISSION" "$PAYMENT" 's/(WALLET_TRANSACTION_TYPE_COMMISSION = )4;/\114;/' "COMMISSION"
renum "ChargeSellerFeeRequest.amount" "$PAYMENT" '/^message ChargeSellerFeeRequest /,/^}/ s/(amount = )2;/\119;/' "amount"
renum "term_days" "$PROMOTION" '/^message Plan /,/^}/ s/(term_days = )5;/\119;/' "term_days"
renum "impression_token" "$PROMOTION" '/^message SponsoredSlot /,/^}/ s/(impression_token = )3;/\119;/' "impression_token"
renum "PAUSED_BUDGET" "$PROMOTION" 's/(AD_CAMPAIGN_STATUS_PAUSED_BUDGET = )4;/\114;/' "PAUSED_BUDGET"

built=""
for r in $UNION; do
  if [ ! -d "$ROOT/$r/proto" ]; then echo "FAIL buf build $r: proto dir missing"; fail=1; continue; fi
  if (cd "$ROOT/$r/proto" && buf build >"$tmp/build.err" 2>&1); then echo "ok   buf build $r"; built="$built $r"
  else echo "FAIL buf build $r: $(head -3 "$tmp/build.err" | tr '\n' ' ')"; fail=1; fi
done
missing=""
for r in $UNION; do case " $built " in *" $r "*) ;; *) missing="$missing $r";; esac; done
if [ -n "$missing" ]; then echo "FAIL build list incomplete, missing:$missing"; fail=1
else echo "ok   built all $(echo "$UNION" | wc -l | tr -d ' ') repos in the union"; fi

if (cd "$SRC_MOD" && buf lint) >"$tmp/lint.out" 2>&1; then echo "ok   buf lint"; else echo "FAIL buf lint: $(head -3 "$tmp/lint.out" | tr '\n' ' ')"; fail=1; fi
if (cd "$SRC_MOD" && buf breaking --against "$ROOT/.git#branch=$BASE_REF,subdir=platform-core/packages/proto") >"$tmp/brk.out" 2>&1; then echo "ok   buf breaking vs $BASE_REF"
else echo "FAIL buf breaking vs $BASE_REF: $(head -3 "$tmp/brk.out" | tr '\n' ' ')"; fail=1; fi
exit $fail
