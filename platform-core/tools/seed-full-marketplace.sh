#!/usr/bin/env bash
# ==============================================================================
# SEED FULL MARKETPLACE SCRIPT — SHOPEE CLONE (20 REAL HIGH-QUALITY PRODUCTS)
# ==============================================================================
set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8080}"
DEFAULT_PASSWORD="pass123"

echo "======================================================================"
echo "🛍️ Starting Full Marketplace Seeding for Shopee Platform: $GW"
echo "======================================================================"

if ! curl -sf "$GW/healthz" >/dev/null; then
  echo "❌ Error: Gateway at $GW is not responding on /healthz"
  exit 1
fi
echo "✓ Gateway is online and healthy"

# Clean old records
echo "· Cleaning old tables in database and OpenSearch..."
docker exec $(docker ps -q -f name=postgres-listing) psql -U listing_svc -d listing_db -c "TRUNCATE TABLE listing_variants, listings CASCADE;" >/dev/null 2>&1 || true
docker exec $(docker ps -q -f name=postgres-engagement) psql -U engagement_svc -d engagement_db -c "TRUNCATE TABLE reviews, favorites, listing_stats CASCADE;" >/dev/null 2>&1 || true
curl -s -X POST 'http://localhost:9200/listings/_delete_by_query' -H 'Content-Type: application/json' -d '{"query": {"match_all": {}}}' >/dev/null 2>&1 || true

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

echo "👤 1. Authenticating Shopee Sellers & Buyers..."
SELLER_TECH_TOKEN=$(register_and_login "shopee_tech_mall" "seller")
SELLER_FASHION_TOKEN=$(register_and_login "shopee_fashion_mall" "seller")
BUYER_TOKEN=$(register_and_login "khach_hang_shopee" "buyer")
echo "✓ Accounts ready"

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
    echo "  ✓ ID: $id ($name)"
    echo "$id"
  else
    echo "  ❌ Failed to create $name: $res" >&2
    return 1
  fi
}

echo ""
echo "📦 2. Seeding 20 High-Quality Authentic E-Commerce Products..."

# 1. iPhone 15 Pro Max
ID1=$(create_product "$SELLER_TECH_TOKEN" "iPhone 15 Pro Max" '{
  "listing": {
    "title": "Điện thoại Apple iPhone 15 Pro Max 256GB - Chính hãng VN/A (Khung viền Titan, Camera 5x)",
    "description": "Siêu phẩm iPhone 15 Pro Max thiết kế khung viền Titan chuẩn hàng không vũ trụ siêu bền nhẹ. Trang bị chip Apple A17 Pro tiến trình 3nm mang lại hiệu năng gaming đỉnh cao, camera telephoto 5x sắc nét và cổng kết nối USB-C tốc độ cao. Bảo hành 12 tháng chính hãng tại tất cả TTBH Apple ủy quyền trên toàn quốc.",
    "price": 29990000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 150,
    "image_keys": [
      "https://images.unsplash.com/photo-1695048133142-1a20484d2569?w=800&auto=format&fit=crop&q=80",
      "https://images.unsplash.com/photo-1510557880182-3d4d3cba35a5?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Titan Tự Nhiên / 256GB", "sku": "IP15PM-NAT-256", "price": 29990000, "stock": 50},
      {"name": "Titan Xanh / 256GB", "sku": "IP15PM-BLU-256", "price": 29990000, "stock": 50},
      {"name": "Titan Đen / 512GB", "sku": "IP15PM-BLK-512", "price": 34990000, "stock": 50}
    ]
  }
}')

# 2. MacBook Pro M3
ID2=$(create_product "$SELLER_TECH_TOKEN" "MacBook Pro 14 M3" '{
  "listing": {
    "title": "Laptop Apple MacBook Pro 14 inch M3 Pro (18GB Unified RAM / 512GB SSD) - Hàng Chính Hãng",
    "description": "MacBook Pro 14 inch chip Apple M3 Pro thế hệ mới kiến trúc đồ họa tân tiến, đáp ứng mượt mà các tác vụ render video 4K, đồ họa 3D và lập trình AI. Màn hình Liquid Retina XDR độ sáng tối đa 1600 nits, thời lượng pin ấn tượng lên đến 18 tiếng.",
    "price": 49990000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 45,
    "image_keys": [
      "https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=800&auto=format&fit=crop&q=80",
      "https://images.unsplash.com/photo-1611186871348-b1ce696e52c9?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Space Black / 18GB/512GB", "sku": "MBP14-M3P-BLK", "price": 49990000, "stock": 25},
      {"name": "Silver / 18GB/1TB", "sku": "MBP14-M3P-SLV-1T", "price": 56990000, "stock": 20}
    ]
  }
}')

# 3. Tai nghe Sony WH-1000XM5
ID3=$(create_product "$SELLER_TECH_TOKEN" "Sony WH-1000XM5" '{
  "listing": {
    "title": "Tai Nghe Chống Ồn Cao Cấp Sony WH-1000XM5 Không Dây - Chống Ồn Chủ Động ANC Đỉnh Cao",
    "description": "Công nghệ chống ồn số 1 thế giới với 2 bộ xử lý và 8 micro chuyên biệt. Chất âm chuẩn Hi-Res Audio Wireless với màng loa carbon 30mm, thời lượng pin 30 giờ liên tục kèm sạc nhanh 3 phút nghe được 3 giờ.",
    "price": 6990000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 80,
    "image_keys": [
      "https://images.unsplash.com/photo-1546435770-a3e426bf472b?w=800&auto=format&fit=crop&q=80",
      "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Đen Nhám Cổ Điển", "sku": "SONY-XM5-BLK", "price": 6990000, "stock": 50},
      {"name": "Bạc Bạch Kim", "sku": "SONY-XM5-SLV", "price": 6990000, "stock": 30}
    ]
  }
}')

# 4. Apple Watch Series 9
ID4=$(create_product "$SELLER_TECH_TOKEN" "Apple Watch Series 9" '{
  "listing": {
    "title": "Đồng Hồ Thông Minh Apple Watch Series 9 GPS 45mm Viền Nhôm Dây Cao Su",
    "description": "Màn hình Retina siêu sáng 2000 nits, chip S9 SiP với tính năng cử chỉ Chạm Hai Lần (Double Tap). Cảm biến đo điện tâm đồ ECG, nồng độ oxy trong máu SpO2, theo dõi giấc ngủ và phát hiện va chạm tự động.",
    "price": 9490000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 60,
    "image_keys": [
      "https://images.unsplash.com/photo-1579586337278-3befd40fd17a?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Nhôm Midnight 45mm", "sku": "AW9-MID-45", "price": 9490000, "stock": 30},
      {"name": "Nhôm Starlight 45mm", "sku": "AW9-STA-45", "price": 9490000, "stock": 30}
    ]
  }
}')

# 5. Củ Sạc Anker GaN 65W
ID5=$(create_product "$SELLER_TECH_TOKEN" "Củ Sạc Anker GaN 65W" '{
  "listing": {
    "title": "Củ Sạc Nhanh Anker GaNPrime 65W 3 Cổng (2 Type-C + 1 USB-A) - Nhỏ Gọn Siêu Nhanh",
    "description": "Công nghệ sạc nhanh GaN III độc quyền từ Anker, công suất 65W sạc cùng lúc MacBook Air, iPhone và Apple Watch an toàn. Công nghệ Dynamic Power Distribution tự động phân bổ nguồn điện thông minh.",
    "price": 890000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 200,
    "image_keys": [
      "https://images.unsplash.com/photo-1622445262464-84b1456045b6?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Đen Phantom", "sku": "ANKER-65W-BLK", "price": 890000, "stock": 120},
      {"name": "Trắng Ngọc Trai", "sku": "ANKER-65W-WHT", "price": 890000, "stock": 80}
    ]
  }
}')

# 6. Bàn Phím Keychron K2 Pro
ID6=$(create_product "$SELLER_TECH_TOKEN" "Keychron K2 Pro" '{
  "listing": {
    "title": "Bàn Phím Cơ Không Dây Keychron K2 Pro QMK/VIA Hot-swap RGB (Layout 75%)",
    "description": "Bàn phím cơ layout 75% gọn gàng hỗ trợ lập trình keymap qua QMK/VIA. Kết nối Bluetooth 5.1 cùng lúc 3 thiết bị và Type-C. Switch Keychron K Pro đã được lube sẵn từ nhà máy cho cảm giác gõ cực êm.",
    "price": 1950000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 90,
    "image_keys": [
      "https://images.unsplash.com/photo-1587829741301-dc798b83add3?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Red Switch (Linear êm)", "sku": "KEY-K2-RED", "price": 1950000, "stock": 45},
      {"name": "Brown Switch (Tactile)", "sku": "KEY-K2-BRW", "price": 1950000, "stock": 45}
    ]
  }
}')

# 7. Chuột Logitech MX Master 3S
ID7=$(create_product "$SELLER_TECH_TOKEN" "Logitech MX Master 3S" '{
  "listing": {
    "title": "Chuột Không Dây Công Thái Học Logitech MX Master 3S Quiet Clicks - Cảm Biến 8000 DPI",
    "description": "Chuột công thái học cao cấp cho dân lập trình và thiết kế. Cảm biến Darkfield 8000 DPI di chuyển mượt trên mặt kính, con lăn điện từ MagSpeed cuộn 1000 dòng/giây và nút bấm yên tĩnh giảm 90% tiếng ồn.",
    "price": 2190000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-electronics",
    "stock": 110,
    "image_keys": [
      "https://images.unsplash.com/photo-1615663245857-ac93bb7c39e7?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Đen Graphite", "sku": "LOGI-MX3S-BLK", "price": 2190000, "stock": 65},
      {"name": "Xám Pale Grey", "sku": "LOGI-MX3S-GRY", "price": 2190000, "stock": 45}
    ]
  }
}')

# 8. Áo Khoác Gió Yody Nam
ID8=$(create_product "$SELLER_FASHION_TOKEN" "Áo Khoác Gió Yody" '{
  "listing": {
    "title": "Áo Khoác Gió Thể Thao Nam Yody 2 Lớp Chống Nước Cản Gió - Form Dáng Trẻ Trung",
    "description": "Chất liệu vải dù cao cấp công nghệ tráng bạc cản gió và chống thấm nước mưa phùn. Mũ trùm đầu tháo rời linh hoạt, khóa kéo YKK bền bỉ và form dáng thể thao năng động dễ phối đồ.",
    "price": 399000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-fashion",
    "stock": 250,
    "image_keys": [
      "https://images.unsplash.com/photo-1548883354-7622d03aca27?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Xanh Navy - Size M", "sku": "YODY-WIND-NVY-M", "price": 399000, "stock": 80},
      {"name": "Xanh Navy - Size L", "sku": "YODY-WIND-NVY-L", "price": 399000, "stock": 90},
      {"name": "Đen Basic - Size L", "sku": "YODY-WIND-BLK-L", "price": 399000, "stock": 80}
    ]
  }
}')

# 9. Áo Thun Coolmate Cotton
ID9=$(create_product "$SELLER_FASHION_TOKEN" "Áo Thun Coolmate" '{
  "listing": {
    "title": "Áo Thun Nam Cổ Tròn Coolmate Cotton Compact 100% Chống Nhăn Co Giãn Thoáng Mát",
    "description": "Chất liệu sợi Cotton Compact siêu dai mềm mịn, không xù lông và thấm hút mồ hôi vượt trội. Form Regular Fit chuẩn người Việt Nam, đường may tỉ mỉ chắc chắn thích hợp mặc hàng ngày.",
    "price": 179000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-fashion",
    "stock": 300,
    "image_keys": [
      "https://images.unsplash.com/photo-1521572267360-ee0c2909d518?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Trắng - Size L", "sku": "COOL-TSHIRT-WHT-L", "price": 179000, "stock": 150},
      {"name": "Đen - Size L", "sku": "COOL-TSHIRT-BLK-L", "price": 179000, "stock": 150}
    ]
  }
}')

# 10. Váy Hoa Nhí Nữ Vintage
ID10=$(create_product "$SELLER_FASHION_TOKEN" "Váy Hoa Nhí Vintage" '{
  "listing": {
    "title": "Váy Hoa Nhí Nữ Dáng Dài Vintage Phong Cách Hàn Quốc Dịu Dàng - Hàng Thiết Kế",
    "description": "Váy voan tơ 2 lớp mềm mại bay bổng với họa tiết hoa nhí xinh xắn. Thiết kế cổ vuông tôn xương quai xanh, tay phồng bo chun nhẹ nhàng và chiết eo thon gọn phù hợp đi chơi, dạo phố, chụp ảnh.",
    "price": 289000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-fashion",
    "stock": 120,
    "image_keys": [
      "https://images.unsplash.com/photo-1572804013309-59a88b7e92f1?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Hoa Vàng Nhạt - Size S", "sku": "DRESS-YLW-S", "price": 289000, "stock": 60},
      {"name": "Hoa Vàng Nhạt - Size M", "sku": "DRESS-YLW-M", "price": 289000, "stock": 60}
    ]
  }
}')

# 11. Chân Váy Tennis Xếp Ly
ID11=$(create_product "$SELLER_FASHION_TOKEN" "Chân Váy Tennis" '{
  "listing": {
    "title": "Chân Váy Xếp Ly Tennis Nữ Lưng Cao Có Quần Bảo Hộ Bên Trong Tôn Dáng",
    "description": "Chân váy tennis chất liệu tuyết mưa cao cấp đứng form, nếp gấp xếp ly sắc nét không lo bị bung sau khi giặt. Kèm quần lót bảo hộ an toàn bên trong cho bạn gái tự tin vận động.",
    "price": 165000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-fashion",
    "stock": 180,
    "image_keys": [
      "https://images.unsplash.com/photo-1583496661160-fb5886a0aaaa?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Trắng - Size S", "sku": "SKIRT-WHT-S", "price": 165000, "stock": 90},
      {"name": "Đen - Size S", "sku": "SKIRT-BLK-S", "price": 165000, "stock": 90}
    ]
  }
}')

# 12. Nike Air Force 1
ID12=$(create_product "$SELLER_FASHION_TOKEN" "Nike Air Force 1" '{
  "listing": {
    "title": "Giày Thể Thao Sneaker Nam Nữ Nike Air Force 1 07 All-White - Hàng Chính Hãng",
    "description": "Đôi giày sneaker huyền thoại của Nike với tone màu trắng tinh tế dễ phối đồ. Đệm khí Nike Air êm ái đàn hồi cao, đế ngoài cao su rãnh xoay bám đường tuyệt vời và form dáng kinh điển vượt thời gian.",
    "price": 2690000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-fashion",
    "stock": 100,
    "image_keys": [
      "https://images.unsplash.com/photo-1595950653106-6c9ebd614d3a?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Size 40 EU (25cm)", "sku": "NIKE-AF1-40", "price": 2690000, "stock": 35},
      {"name": "Size 41 EU (26cm)", "sku": "NIKE-AF1-41", "price": 2690000, "stock": 35},
      {"name": "Size 42 EU (26.5cm)", "sku": "NIKE-AF1-42", "price": 2690000, "stock": 30}
    ]
  }
}')

# 13. Adidas Ultraboost
ID13=$(create_product "$SELLER_FASHION_TOKEN" "Adidas Ultraboost Light" '{
  "listing": {
    "title": "Giày Chạy Bộ Nam Nữ Adidas Ultraboost Light Siêu Nhẹ Đệm Boost Êm Ái",
    "description": "Thế hệ đệm Boost nhẹ nhất lịch sử với khả năng hoàn trả năng lượng tối đa trên từng bước chạy. Thân giày vải dệt Primeknit+ ôm sát vừa vặn như tất và đế ngoài cao su Continental bền bỉ.",
    "price": 3200000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-fashion",
    "stock": 70,
    "image_keys": [
      "https://images.unsplash.com/photo-1584735935682-2f2b69dff9d2?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Trắng Đen - Size 41", "sku": "ADI-UB-41", "price": 3200000, "stock": 35},
      {"name": "Đen All-Black - Size 42", "sku": "ADI-UB-42", "price": 3200000, "stock": 35}
    ]
  }
}')

# 14. Son Black Rouge Ver 8
ID14=$(create_product "$SELLER_FASHION_TOKEN" "Son Black Rouge Ver 8" '{
  "listing": {
    "title": "Son Kem Lì Black Rouge Air Fit Velvet Tint Ver 8 The Crystal - Mềm Mịn Lâu Trôi",
    "description": "Dòng son kem lì đình đám với chất son velvet mềm xốp, lướt nhẹ trên môi và không gây khô môi. Khả năng bám màu lâu trôi từ 6-8 tiếng với bảng màu thời thượng tôn da cực đỉnh.",
    "price": 185000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-beauty",
    "stock": 250,
    "image_keys": [
      "https://images.unsplash.com/photo-1586495777744-4413f21062fa?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "A06 - Đỏ Gạch Nổi Bật", "sku": "BR-VER8-A06", "price": 185000, "stock": 125},
      {"name": "A12 - Nâu Đỏ Quyến Rũ", "sku": "BR-VER8-A12", "price": 185000, "stock": 125}
    ]
  }
}')

# 15. Kem Chống Nắng La Roche-Posay
ID15=$(create_product "$SELLER_FASHION_TOKEN" "Kem Chống Nắng La Roche-Posay" '{
  "listing": {
    "title": "Kem Chống Nắng Kiểm Soát Dầu La Roche-Posay Anthelios SPF50+ 50ml (Bản Mới)",
    "description": "Kem chống nắng phổ rộng bảo vệ da tối đa trước tia UVA/UVB và ô nhiễm môi trường. Công nghệ Airlicium kiểm soát dầu nhờn đến 12 giờ, chất kem thẩm thấu nhanh không để lại vệt trắng.",
    "price": 395000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-beauty",
    "stock": 160,
    "image_keys": [
      "https://images.unsplash.com/photo-1556228720-195a672e8a03?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Vạch Xanh Lá (Kiểm soát dầu)", "sku": "LRP-SUN-OIL", "price": 395000, "stock": 100},
      {"name": "Vạch Vàng (Dưỡng ẩm da)", "sku": "LRP-SUN-DRY", "price": 395000, "stock": 60}
    ]
  }
}')

# 16. Roborock S8 Pro
ID16=$(create_product "$SELLER_TECH_TOKEN" "Roborock S8 Pro Ultra" '{
  "listing": {
    "title": "Robot Hút Bụi Lau Nhà Tự Động Giặt Giẻ Sấy Khí Nóng Roborock S8 Pro Ultra",
    "description": "Trạm sạc RockDock Ultra tự động đổ rác, giặt và sấy giẻ bằng khí nóng. Lực hút cực đại 6000Pa, chổi lăn kép DuoRoller chống rối và hệ thống lau rung VibraRise 2.0 sạch bóng sàn nhà.",
    "price": 21990000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-appliances",
    "stock": 35,
    "image_keys": [
      "https://images.unsplash.com/photo-1518640467707-6811f4a6ab73?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Trắng Sang Trọng", "sku": "ROBOROCK-S8-WHT", "price": 21990000, "stock": 20},
      {"name": "Đen Huyền Bí", "sku": "ROBOROCK-S8-BLK", "price": 21990000, "stock": 15}
    ]
  }
}')

# 17. Nồi Chiên Philips XXL
ID17=$(create_product "$SELLER_TECH_TOKEN" "Philips XXL HD9650" '{
  "listing": {
    "title": "Nồi Chiên Không Dầu Điện Tử Philips XXL HD9650/90 Dung Tích 7.3L - Công Suất 2225W",
    "description": "Công nghệ Twin TurboStar loại bỏ đến 90% chất béo trong thực phẩm. Lòng nồi dung tích lớn nướng nguyên con gà 1.4kg hoặc 1.4kg khoai tây chiên vàng giòn rụm cho cả gia đình.",
    "price": 4290000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-appliances",
    "stock": 60,
    "image_keys": [
      "https://images.unsplash.com/photo-1585659722983-3a675dabf23d?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Đen Bóng Điện Tử 7.3L", "sku": "PHILIPS-XXL-BLK", "price": 4290000, "stock": 60}
    ]
  }
}')

# 18. Máy Lọc Không Khí Xiaomi
ID18=$(create_product "$SELLER_TECH_TOKEN" "Xiaomi Air Purifier 4 Pro" '{
  "listing": {
    "title": "Máy Lọc Không Khí Thông Minh Xiaomi Smart Air Purifier 4 Pro - Lọc Bụi Mịn PM2.5",
    "description": "Lọc sạch bụi mịn PM2.5, khói phấn hoa và vi khuẩn với tốc độ phân phối không khí sạch 500m3/h cho phòng rộng đến 60m2. Cảm biến laser kép hiển thị chất lượng không khí trên màn hình OLED và app Mi Home.",
    "price": 3890000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-appliances",
    "stock": 80,
    "image_keys": [
      "https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Trắng Tinh Khiết Bản Quốc Tế", "sku": "XIAOMI-AIR-4PRO", "price": 3890000, "stock": 80}
    ]
  }
}')

# 19. Bình Giữ Nhiệt Lock&Lock
ID19=$(create_product "$SELLER_FASHION_TOKEN" "Bình Giữ Nhiệt Lock&Lock" '{
  "listing": {
    "title": "Bình Giữ Nhiệt Lock&Lock Feather Light Thép Không Gỉ 450ml - Giữ Nóng Lạnh 24h",
    "description": "Bình giữ nhiệt siêu nhẹ chỉ 230g làm bằng inox 304 an toàn thực phẩm. Khả năng giữ nóng đến 8 giờ và giữ lạnh đến 24 giờ. Nắp bật 1 chạm có khóa cài an toàn chống rò rỉ nước.",
    "price": 249000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-home",
    "stock": 200,
    "image_keys": [
      "https://images.unsplash.com/photo-1602143407151-7111542de6e8?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Xanh Mint Dịu Mát", "sku": "LOCK-FL-MNT", "price": 249000, "stock": 100},
      {"name": "Đen Nhám Thể Thao", "sku": "LOCK-FL-BLK", "price": 249000, "stock": 100}
    ]
  }
}')

# 20. Xe Đạp Giant ATX 830
ID20=$(create_product "$SELLER_TECH_TOKEN" "Xe Đạp Giant ATX 830" '{
  "listing": {
    "title": "Xe Đạp Thể Thao Địa Hình Giant ATX 830 Khung Hợp Kim Nhôm ALUXX - Phanh Đĩa Dầu",
    "description": "Xe đạp địa hình cao cấp nhập khẩu chính hãng Giant. Khung sườn hợp kim nhôm siêu nhẹ bền bỉ, bộ truyền động Shimano 27 tốc độ chuyển số êm ái mượt mà và phanh đĩa dầu thủy lực an toàn tuyệt đối.",
    "price": 8990000,
    "currency": "VND",
    "status": "LISTING_STATUS_PUBLISHED",
    "category_id": "cat-sports",
    "stock": 30,
    "image_keys": [
      "https://images.unsplash.com/photo-1485965120184-e220f721d03e?w=800&auto=format&fit=crop&q=80"
    ],
    "variants": [
      {"name": "Màu Xanh Đậm - Size M", "sku": "GIANT-ATX830-BLU", "price": 8990000, "stock": 15},
      {"name": "Màu Đen Đỏ - Size L", "sku": "GIANT-ATX830-RED", "price": 8990000, "stock": 15}
    ]
  }
}')

echo ""
echo "⭐ 3. Adding Verified Reviews & Engagement Stats..."

add_review() {
  local token="$1"
  local listing_id="$2"
  local rating="$3"
  local comment="$4"

  curl -s -X POST "$GW/platform.engagement.v1.EngagementService/CreateReview" \
    -H 'Content-Type: application/json' \
    -H "Authorization: bearer $token" \
    -d "{\"listing_id\":\"$listing_id\",\"rating\":$rating,\"comment\":\"$comment\"}" >/dev/null || true

  curl -s -X POST "$GW/platform.engagement.v1.EngagementService/RecordView" \
    -H 'Content-Type: application/json' \
    -d "{\"listing_id\":\"$listing_id\"}" >/dev/null || true

  curl -s -X POST "$GW/platform.engagement.v1.EngagementService/AddFavorite" \
    -H 'Content-Type: application/json' \
    -H "Authorization: bearer $token" \
    -d "{\"listing_id\":\"$listing_id\"}" >/dev/null || true
}

add_review "$BUYER_TOKEN" "$ID1" 5 "Máy nguyên seal mới 100%, màu Titan tự nhiên nhìn thực tế cực kỳ sang trọng. Giao hàng hỏa tốc trong 2h tại Hà Nội. Shop đóng gói rất cẩn thận, 10 sao cho shop!"
add_review "$BUYER_TOKEN" "$ID2" 5 "MacBook Pro M3 Pro render video 4K siêu nhanh, bàn phím gõ êm, màn hình Liquid Retina XDR màu chuẩn đét. Đáng đồng tiền bát gạo!"
add_review "$BUYER_TOKEN" "$ID3" 5 "Chống ồn đỉnh chóp, đeo êm tai không bị cấn, bass đầm chắc và pin trâu kinh khủng. Rất ưng ý!"
add_review "$BUYER_TOKEN" "$ID8" 5 "Áo khoác gió Yody form dáng đẹp, cản gió đi xe máy cực ấm, đường may tỉ mỉ."
add_review "$BUYER_TOKEN" "$ID12" 5 "Air Force 1 All-White chuẩn auth, phối đồ gì cũng đẹp, đế êm đi bộ cả ngày không đau chân."
add_review "$BUYER_TOKEN" "$ID14" 5 "Son Black Rouge màu A12 siêu tôn da, chất son mịn lì như nhung, thơm mùi nho dễ chịu."
add_review "$BUYER_TOKEN" "$ID16" 5 "Roborock S8 Pro tự giặt sấy giẻ siêu sạch, nhà lúc nào cũng bóng loáng, giải phóng sức lao động hoàn toàn!"

echo "✓ Reviews and ratings added successfully!"

echo ""
echo "======================================================================"
echo "🎉 SHOPEE DATASET SEEDING COMPLETED SUCCESSFULLY!"
echo "======================================================================"
echo "📊 Summary:"
echo "  • Total Products:    20 high-quality products across all categories"
echo "  • Categories:        Điện Thoại, Laptop, Thời Trang Nam/Nữ, Mỹ Phẩm, Gia Dụng..."
echo "  • Pricing & Images:  100% realistic retail VND pricing & Unsplash CDN photos"
echo "  • Gateway Base URL:  $GW"
echo "======================================================================"
