#!/usr/bin/env bash
# Seed the marketplace with realistic data through the GATEWAY (so it flows the
# full pipeline: team-domain write → Kafka → team-search index). Idempotent-ish:
# registering an existing shop is ignored; listings are always appended.
#
# Usage: GATEWAY_URL=http://localhost:8080 PER_SHOP=15 tools/seed-marketplace.sh
set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8080}"
PER_SHOP="${PER_SHOP:-15}"
PASSWORD="pass123"
SHOPS=("shop_thanhly" "shop_dienmay" "shop_thoitrang")

# Product name pools by loose category.
declare -a PHONES=("iPhone 13" "iPhone 14 Pro" "Samsung Galaxy S23" "Xiaomi Redmi Note 12" "Oppo Reno8" "Vivo Y36" "Realme C55")
declare -a LAPTOPS=("MacBook Pro M2" "MacBook Air M1" "Dell XPS 13" "Asus ROG Strix" "Lenovo ThinkPad X1" "HP Pavilion 15" "Acer Nitro 5")
declare -a VEHICLES=("Honda Wave Alpha" "Yamaha Exciter 155" "Honda SH Mode" "Xe đạp thể thao Giant" "Vespa Sprint")
declare -a FASHION=("Áo khoác dù nam" "Giày Nike Air Force" "Túi xách nữ da" "Đồng hồ Casio" "Balo laptop chống sốc")
declare -a HOME=("Nồi chiên không dầu Lock&Lock" "Máy lọc nước Kangaroo" "Quạt điều hòa Sunhouse" "Robot hút bụi Xiaomi" "Bếp từ đôi")
declare -a CONDITIONS=("mới 100%" "likenew 99%" "đã qua sử dụng" "chính hãng còn bảo hành" "fullbox")

ALL=("${PHONES[@]}" "${LAPTOPS[@]}" "${VEHICLES[@]}" "${FASHION[@]}" "${HOME[@]}")

login() { # username -> token (register first, ignore if exists)
  local u="$1"
  curl -s "$GW/platform.identity.v1.AuthService/Register" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$u\",\"password\":\"$PASSWORD\",\"role\":\"seller\"}" >/dev/null || true
  curl -s "$GW/platform.identity.v1.AuthService/Login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$u\",\"password\":\"$PASSWORD\"}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4
}

created=0
for shop in "${SHOPS[@]}"; do
  token="$(login "$shop")"
  [ -z "$token" ] && { echo "! could not log in $shop"; continue; }
  for _ in $(seq 1 "$PER_SHOP"); do
    name="${ALL[$((RANDOM % ${#ALL[@]}))]}"
    cond="${CONDITIONS[$((RANDOM % ${#CONDITIONS[@]}))]}"
    title="$name"
    desc="$name - $cond, giá tốt tại $shop"
    price=$(( (RANDOM % 40 + 1) * 500000 ))     # 0.5M .. 20M
    status="LISTING_STATUS_PUBLISHED"
    [ $((RANDOM % 6)) -eq 0 ] && status="LISTING_STATUS_DRAFT"  # ~1/6 drafts
    curl -s -o /dev/null "$GW/platform.listing.v1.ListingService/CreateListing" \
      -H 'Content-Type: application/json' -H "Authorization: bearer $token" \
      -d "{\"listing\":{\"title\":\"$title\",\"description\":\"$desc\",\"price\":$price,\"currency\":\"VND\",\"status\":\"$status\"}}"
    created=$((created + 1))
  done
  echo "seeded $PER_SHOP listings for $shop"
done
echo "done: $created listings created via $GW"
