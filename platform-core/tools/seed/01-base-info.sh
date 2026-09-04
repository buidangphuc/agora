#!/usr/bin/env bash
# ==============================================================================
# SEED MODULE 1: BASE-INFO (Core Accounts, Categories, Listings & Reviews)
# ==============================================================================
set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8080}"
DEFAULT_PASSWORD="pass123"

echo "======================================================================"
echo "📦 [1/3] Seeding BASE-INFO: Core Accounts, Categories, Listings & Reviews"
echo "======================================================================"

if ! curl -sf "$GW/healthz" >/dev/null; then
  echo "❌ Error: Gateway at $GW is not responding on /healthz"
  exit 1
fi

# Clean old records in listing, engagement and search
echo "· Resetting listings, categories, reviews & search indices..."
docker exec $(docker ps -q -f name=postgres-listing) psql -U listing_svc -d listing_db -c "TRUNCATE TABLE listing_variants, listings CASCADE;" >/dev/null 2>&1 || true
docker exec $(docker ps -q -f name=postgres-engagement) psql -U engagement_svc -d engagement_db -c "TRUNCATE TABLE reviews, favorites, listing_stats CASCADE;" >/dev/null 2>&1 || true
curl -s -X POST 'http://localhost:9200/listings/_delete_by_query' -H 'Content-Type: application/json' -d '{"query": {"match_all": {}}}' >/dev/null 2>&1 || true

# Seed 10 Official Categories
docker exec -i $(docker ps -q -f name=postgres-listing) psql -U listing_svc -d listing_db << 'EOF' >/dev/null 2>&1 || true
INSERT INTO categories (id, name, slug, icon_url) VALUES
('cat-dienthoai', 'Điện Thoại & Phụ Kiện', 'dien-thoai-phu-kien', '📱'),
('cat-laptop', 'Máy Tính & Laptop', 'may-tinh-laptop', '💻'),
('cat-thoitrangnam', 'Thời Trang Nam', 'thoi-trang-nam', '👔'),
('cat-thoitrangnu', 'Thời Trang Nữ', 'thoi-trang-nu', '👗'),
('cat-sacdep', 'Sắc Đẹp & Mỹ Phẩm', 'sac-dep-my-pham', '💄'),
('cat-giadung', 'Thiết Bị Điện Gia Dụng', 'thiet-bi-dien-gia-dung', '🔌'),
('cat-nhacua', 'Nhà Cửa & Đời Sống', 'nha-cua-doi-song', '🏠'),
('cat-giaydep', 'Giày Dép Nam Nữ', 'giay-dep-nam-nu', '👟'),
('cat-thethao', 'Thể Thao & Dã Ngoại', 'the-thao-da-ngoai', '⚽'),
('cat-amthanh', 'Thiết Bị Âm Thanh', 'thiet-bi-am-thanh', '🎧')
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, icon_url = EXCLUDED.icon_url;
EOF
echo "✓ 10 official categories seeded"

register_and_login() {
  local username="$1"
  local role="$2"

  curl -s -X POST "$GW/platform.identity.v1.AuthService/Register" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$username\",\"password\":\"$DEFAULT_PASSWORD\",\"role\":\"$role\"}" >/dev/null || true

  local token
  token=$(curl -s -X POST "$GW/platform.identity.v1.AuthService/Login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$username\",\"password\":\"$DEFAULT_PASSWORD\"}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4 || true)

  if [ -z "$token" ]; then
    echo "❌ Failed to obtain token for $username" >&2
    return 1
  fi
  echo "$token"
}

echo "· Authenticating Sellers & Buyers..."
SELLER_TECH_TOKEN=$(register_and_login "shopee_tech_mall" "seller")
SELLER_FASHION_TOKEN=$(register_and_login "shopee_fashion_mall" "seller")
BUYER_TOKEN=$(register_and_login "khach_hang_shopee" "buyer")
echo "✓ Accounts authenticated"

create_product() {
  local token="$1"
  local name="$2"
  local json_payload="$3"

  local res
  res=$(curl -s -X POST "$GW/platform.listing.v1.ListingService/CreateListing" \
    -H 'Content-Type: application/json' \
    -H "Authorization: bearer $token" \
    -d "$json_payload")

  local id
  id=$(echo "$res" | grep -o '"id":"[^"]*"' | head -n 1 | cut -d'"' -f4 || true)
  if [ -n "$id" ]; then
    echo "  ✓ Base Product Created: $name" >&2
    echo "$id"
  else
    echo "  ❌ Failed to create $name: $res" >&2
    return 1
  fi
}

add_review() {
  local listing_id="$1"
  local rating="$2"
  local comment="$3"
  curl -s -X POST "$GW/platform.engagement.v1.EngagementService/CreateReview" \
    -H 'Content-Type: application/json' \
    -H "Authorization: bearer $BUYER_TOKEN" \
    -d "{\"listingId\":\"$listing_id\",\"rating\":$rating,\"comment\":\"$comment\"}" >/dev/null || true
}

# 1. iPhone 15 Pro Max
P1=$(create_product "$SELLER_TECH_TOKEN" "iPhone 15 Pro Max" '{
  "listing": {
    "title": "Điện thoại Apple iPhone 15 Pro Max 256GB - Chính hãng VN/A (Khung viền Titan, Camera 5x)",
    "description": "Siêu phẩm iPhone 15 Pro Max thiết kế khung viền Titan chuẩn hàng không vũ trụ siêu bền nhẹ. Trang bị chip Apple A17 Pro tiến trình 3nm mang lại hiệu năng gaming đỉnh cao, camera telephoto 5x sắc nét và cổng kết nối USB-C tốc độ cao. Bảo hành 12 tháng chính hãng tại tất cả TTBH Apple ủy quyền trên toàn quốc.",
    "price": 29990000,
    "currency": "VND",
    "categoryId": "cat-dienthoai",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 150,
    "images": [
      "https://images.unsplash.com/photo-1695048133142-1a20484d2569?w=800&auto=format&fit=crop&q=80",
      "https://images.unsplash.com/photo-1695048065053-cfd07efbe868?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Titan Tự Nhiên / 256GB", "sku": "IP15PM-NAT-256", "price": 29990000, "stock": 50},
      {"name": "Titan Xanh / 256GB", "sku": "IP15PM-BLU-256", "price": 29990000, "stock": 50},
      {"name": "Titan Đen / 512GB", "sku": "IP15PM-BLK-512", "price": 34990000, "stock": 50}
    ]
  }
}')
add_review "$P1" 5 "Máy nguyên seal, bảo hành Apple VN/A chuẩn đét. Giao hàng cực nhanh chỉ trong 2 tiếng!"

# 2. MacBook Pro M3
P2=$(create_product "$SELLER_TECH_TOKEN" "MacBook Pro 14 inch M3" '{
  "listing": {
    "title": "Laptop Apple MacBook Pro 14 inch M3 Pro (18GB Unified RAM / 512GB SSD) - Space Black Chính Hãng",
    "description": "MacBook Pro 14 inch mới nhất trang bị chip M3 Pro với kiến trúc GPU đột phá Dynamic Caching và Ray Tracing phần cứng. Màn hình Liquid Retina XDR độ sáng 1600 nits đỉnh cao. Thời lượng pin kỷ lục lên tới 18 giờ liên tục. Màu Space Black sang trọng chống bám vân tay.",
    "price": 49990000,
    "currency": "VND",
    "categoryId": "cat-laptop",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 45,
    "images": [
      "https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P2" 5 "Màu Space Black cực đẹp, màn hình XDR siêu nét, render video 4K mượt mà không nóng máy."

# 3. Tai Nghe Sony WH-1000XM5
P3=$(create_product "$SELLER_TECH_TOKEN" "Tai Nghe Chống Ồn Sony WH-1000XM5" '{
  "listing": {
    "title": "Tai Nghe Chống Ồn Cao Cấp Sony WH-1000XM5 Không Dây - Chống Ồn Chủ Động Flagship",
    "description": "Sony WH-1000XM5 định chuẩn trải nghiệm âm thanh không dây với bộ xử lý chống ồn tích hợp V1 và HD QN1. 8 micro đa hướng thu âm giọng nói trong trẻo tuyệt đối. Thời lượng pin 30 giờ và sạc nhanh 3 phút dùng 3 giờ.",
    "price": 6990000,
    "currency": "VND",
    "categoryId": "cat-amthanh",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 80,
    "images": [
      "https://images.unsplash.com/photo-1546435770-a3e426bf472b?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P3" 5 "Chống ồn đỉnh chóp, đi máy bay hay ngồi quán cafe ồn ào bật lên là tĩnh lặng hoàn toàn."

# 4. Robot Hút Bụi Roborock S8 Pro Ultra
P4=$(create_product "$SELLER_TECH_TOKEN" "Robot Hút Bụi Roborock S8 Pro Ultra" '{
  "listing": {
    "title": "Robot Hút Bụi Lau Nhà Tự Động Giặt Giẻ Sấy Khí Nóng Roborock S8 Pro Ultra - Lực Hút 6000Pa",
    "description": "Đỉnh cao robot hút bụi tự động với trạm sạc RockDock Ultra: Tự giặt giẻ, sấy khô bằng khí nóng, tự đổ rác 7 tuần, tự bơm nước sạch. Hệ thống chổi xoắn kép DuoRoller Riser và lau rung sóng âm VibraRise 2.0 lực hút cực mạnh 6000Pa.",
    "price": 21990000,
    "currency": "VND",
    "categoryId": "cat-giadung",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 35,
    "images": [
      "https://images.unsplash.com/photo-1518640467707-6811f4a6ab73?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P4" 5 "Nhà sạch bóng kin kít, tự giặt sấy giẻ thơm tho không hề bị hôi ẩm."

# 5. Máy Lọc Không Khí Xiaomi Smart Air Purifier 4 Pro
P5=$(create_product "$SELLER_TECH_TOKEN" "Máy Lọc Không Khí Xiaomi 4 Pro" '{
  "listing": {
    "title": "Máy Lọc Không Khí Thông Minh Xiaomi Smart Air Purifier 4 Pro - Lọc Bụi Mịn PM2.5 Kháng Khuẩn 99.99%",
    "description": "Xiaomi Air Purifier 4 Pro lọc sạch không khí phòng 60m2 chỉ trong 15 phút với CADR 500m3/h. Cảm biến laser kép phát hiện hạt bụi mịn PM2.5 và PM10 theo thời gian thực. Điều khiển qua app Mi Home và giọng nói Google Assistant.",
    "price": 3890000,
    "currency": "VND",
    "categoryId": "cat-giadung",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 90,
    "images": [
      "https://images.unsplash.com/photo-1585338107529-13afc5f02586?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P5" 5 "Hút bụi mịn rất tốt, phòng ngủ hết hẳn mùi ẩm mốc, app điều khiển tiện lợi."

# 6. Áo Thun Coolmate Cotton Compact
P6=$(create_product "$SELLER_FASHION_TOKEN" "Áo Thun Nam Cổ Tròn Coolmate" '{
  "listing": {
    "title": "Áo Thun Nam Cổ Tròn Coolmate Cotton Compact 100% Chống Nhăn Co Giãn Thoáng Khí",
    "description": "Áo thun nam Coolmate sử dụng sợi Cotton Compact cao cấp chải kỹ 2 lần, bền gấp 2 lần so với cotton thông thường. Form dáng Regular-fit tôn dáng, thấm hút mồ hôi tối ưu, công nghệ may flatlock êm ái không gây cấn da.",
    "price": 1790000,
    "currency": "VND",
    "categoryId": "cat-thoitrangnam",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 300,
    "images": [
      "https://images.unsplash.com/photo-1521572267360-ee0c2909d518?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P6" 5 "Chất vải mềm mịn, giặt máy không bị xù lông hay bai dão, 10 điểm cho thương hiệu Việt!"

# 7. Váy Hoa Nhí Nữ Vintage
P7=$(create_product "$SELLER_FASHION_TOKEN" "Váy Hoa Nhí Nữ Dáng Dài Vintage" '{
  "listing": {
    "title": "Váy Hoa Nhí Nữ Dáng Dài Vintage Phong Cách Hàn Quốc - Vải Voan Tơ Cao Cấp 2 Lớp",
    "description": "Váy đầm hoa nhí vintage thiết kế chiết eo tôn dáng nhẹ nhàng, tay bồng nữ tính, chất vải voan tơ mềm rủ bay bổng kèm lớp lót lụa kín đáo mềm mại. Thích hợp đi tiệc, đi chơi, chụp ảnh du lịch biển xinh xắn.",
    "price": 289000,
    "currency": "VND",
    "categoryId": "cat-thoitrangnu",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 120,
    "images": [
      "https://images.unsplash.com/photo-1572804013309-59a88b7e92f1?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P7" 5 "Váy xinh lắm, mặc lên dáng như nàng thơ, đường may cẩn thận."

# 8. Kem Chống Nắng La Roche-Posay Anthelios
P8=$(create_product "$SELLER_FASHION_TOKEN" "Kem Chống Nắng La Roche-Posay Anthelios" '{
  "listing": {
    "title": "Kem Chống Nắng Kiểm Soát Dầu La Roche-Posay Anthelios UVmune 400 Oil Control Gel-Cream 50ml",
    "description": "Kem chống nắng kiểm soát dầu số 1 thế giới La Roche-Posay Anthelios với màng lọc Mexoryl 400 độc quyền chống tia UVA dài. Công nghệ Airlicium hấp thụ bã nhờn tức thì, kiềm dầu đến 12h, không bóng nhờn, không vệt trắng.",
    "price": 395000,
    "currency": "VND",
    "categoryId": "cat-sacdep",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 250,
    "images": [
      "https://images.unsplash.com/photo-1556228720-195a672e8a03?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P8" 5 "Kem kiềm dầu siêu đỉnh, không hề bị bí da hay lên mụn, dùng chai thứ 4 rồi!"

# 9. Giày Sneaker Nike Air Force 1
P9=$(create_product "$SELLER_FASHION_TOKEN" "Giày Thể Thao Nike Air Force 1 07" '{
  "listing": {
    "title": "Giày Thể Thao Sneaker Nam Nữ Nike Air Force 1 07 All-White - Hàng Chính Hãng",
    "description": "Biểu tượng thời trang đường phố bất hủ Nike Air Force 1 07 với thân giày da thật cao cấp, đế đệm khí Nike Air êm ái cả ngày dài và đế ngoài cao su rãnh xoay bám đường hoàn hảo.",
    "price": 2690000,
    "currency": "VND",
    "categoryId": "cat-giaydep",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 95,
    "images": [
      "https://images.unsplash.com/photo-1595950653106-6c9ebd614d3a?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P9" 5 "Giày chính hãng form cứng cáp, mang cực kỳ êm chân và dễ phối đồ."

# 10. Xe Đạp Giant ATX 830
P10=$(create_product "$SELLER_TECH_TOKEN" "Xe Đạp Thể Thao Địa Hình Giant ATX 830" '{
  "listing": {
    "title": "Xe Đạp Thể Thao Địa Hình Giant ATX 830 Khung Hợp Kim Nhôm ALUXX - Phanh Đĩa Dầu Thủy Lực",
    "description": "Dòng xe địa hình Giant ATX 830 khung nhôm siêu nhẹ ALUXX độc quyền của Giant. Bộ truyền động Shimano Deore 27 tốc độ chuyển số mượt mà, phanh đĩa dầu thủy lực an toàn trong mọi điều kiện thời tiết.",
    "price": 8990000,
    "currency": "VND",
    "categoryId": "cat-thethao",
    "status": "LISTING_STATUS_PUBLISHED",
    "stock": 25,
    "images": [
      "https://images.unsplash.com/photo-1485965120184-e220f721d03e?w=800&auto=format&fit=crop&q=80"
    ]
  }
}')
add_review "$P10" 5 "Xe đạp rất chắc chắn, giảm xóc êm, sang số nhẹ nhàng leo dốc cực khỏe."

echo "✅ BASE-INFO Seeding Completed Successfully!"
