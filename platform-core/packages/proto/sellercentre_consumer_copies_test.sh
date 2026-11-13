#!/usr/bin/env bash
set -u
here="$(cd "$(dirname "$0")" && pwd)"
ROOT="${REPO_ROOT:-$(cd "$here/../../.." && pwd)}"
SRC_REL="platform-core/packages/proto/platform"
SRC_MOD="$ROOT/platform-core/packages/proto"
BASE_REF="${BASE_REF:-origin/main}"
FE_GEN="$ROOT/fe-platform/packages/api-client/src/generated/platform"

PROMOTION="platform/promotion/v1/promotion.proto"
ORDER="platform/order/v1/order.proto"
LISTING="platform/listing/v1/listing.proto"

PROMOTION_REPOS="team-analytics team-frontend-admin team-frontend-seller team-frontend team-gateway team-order team-promotion"
ORDER_REPOS="team-analytics team-chat team-domain team-engagement team-frontend-admin team-frontend-seller team-frontend team-gateway team-notification team-order team-payment team-promotion team-referral team-search"
LISTING_REPOS="team-ai team-analytics team-chat team-domain team-engagement team-frontend-admin team-frontend-seller team-frontend team-gateway team-identity team-notification team-order team-payment team-promotion team-search"

pairs() {
  local r
  for r in $PROMOTION_REPOS; do echo "$PROMOTION $r"; done
  for r in $ORDER_REPOS; do echo "$ORDER $r"; done
  for r in $LISTING_REPOS; do echo "$LISTING $r"; done
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

if [ "$NPAIRS" != 36 ]; then echo "FAIL pair table has $NPAIRS entries, expected 36 (7+14+15)"; fail=1; fi

PSRC="$SRC_MOD/$PROMOTION"; OSRC="$SRC_MOD/$ORDER"; LSRC="$SRC_MOD/$LISTING"
declare_ok=1
need() { grep -Eq "$2" "$1" || { echo "FAIL source $(basename "$1") lacks: $3"; declare_ok=0; }; }
for rpc in ListMyCampaigns CancelMyCampaign ListMyShopVouchers DeactivateMyShopVoucher; do
  need "$PSRC" "rpc $rpc\\($rpc""Request\\) returns \\($rpc""Response\\);" "rpc $rpc"
done
need "$PSRC" '^[[:space:]]*string seller_id = 11;' "FlashSaleCampaign.seller_id = 11"
need "$OSRC" 'rpc AddBundleToCart\(AddBundleToCartRequest\) returns \(AddBundleToCartResponse\);' "rpc AddBundleToCart"
need "$OSRC" '^message OrderStatusCount ' "message OrderStatusCount"
need "$LSRC" 'repeated ListingStatus statuses = 2;' "ListMyListingsRequest.statuses = 2"
need "$LSRC" 'string query = 3;' "ListMyListingsRequest.query = 3"
if [ "$declare_ok" = 1 ]; then echo "ok   source declares the contract"; else fail=1; fi

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
mk; printf 'x' >> "$tmp/t/team-order/proto/$ORDER"; rm "$tmp/t/team-frontend/proto/$PROMOTION"
if out="$(check_copies "$tmp/t")"; then echo "FAIL drift and missing copy accepted"; fail=1
elif echo "$out" | grep -q "team-order/proto/$ORDER" && echo "$out" | grep -q "team-frontend/proto/$PROMOTION"; then echo "ok   drifted and missing copy rejected and named"
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
renum "FlashSaleCampaign.seller_id" "$PROMOTION" '/^message FlashSaleCampaign /,/^}/ s/(seller_id = )11;/\119;/' "seller_id"
renum "ListMyListingsRequest.statuses" "$LISTING" '/^message ListMyListingsRequest /,/^}/ s/(statuses = )2;/\19;/' "statuses"
renum "ListMyListingsRequest.query" "$LISTING" '/^message ListMyListingsRequest /,/^}/ s/(query = )3;/\19;/' "query"

built=""
for r in $UNION; do
  if [ ! -d "$ROOT/$r/proto" ]; then echo "FAIL buf lint $r: proto dir missing"; fail=1; continue; fi
  paths=""
  for rel in $(pairs | awk -v r="$r" '$2==r{print $1}'); do paths="$paths --path $rel"; done
  if (cd "$ROOT/$r/proto" && buf lint $paths >"$tmp/lint.err" 2>&1); then echo "ok   buf lint $r"; built="$built $r"
  else echo "FAIL buf lint $r: $(head -3 "$tmp/lint.err" | tr '\n' ' ')"; fail=1; fi
done
missing=""
for r in $UNION; do case " $built " in *" $r "*) ;; *) missing="$missing $r";; esac; done
if [ -n "$missing" ]; then echo "FAIL lint list incomplete, missing:$missing"; fail=1
else echo "ok   linted all $(echo "$UNION" | wc -l | tr -d ' ') repos in the union"; fi

gen_has() {
  local f="$1" sym="$2"
  grep -q "$sym" "$FE_GEN/$f" 2>/dev/null || { echo "FAIL fe-platform generated $f lacks $sym"; fail=1; return 1; }
}
g=1
gen_has order/v1/order_connect.ts addBundleToCart || g=0
gen_has order/v1/order_pb.ts OrderStatusCount || g=0
gen_has listing/v1/listing_pb.ts "statuses" || g=0
gen_has listing/v1/listing_pb.ts "query" || g=0
[ "$g" = 1 ] && echo "ok   fe-platform api-client generated TS exposes the new contract"

if (cd "$SRC_MOD" && buf lint) >"$tmp/lint.out" 2>&1; then echo "ok   buf lint source"; else echo "FAIL buf lint source: $(head -3 "$tmp/lint.out" | tr '\n' ' ')"; fail=1; fi
if (cd "$SRC_MOD" && buf breaking --against "$ROOT/.git#branch=$BASE_REF,subdir=platform-core/packages/proto") >"$tmp/brk.out" 2>&1; then echo "ok   buf breaking vs $BASE_REF"
else echo "FAIL buf breaking vs $BASE_REF: $(head -3 "$tmp/brk.out" | tr '\n' ' ')"; fail=1; fi
exit $fail
