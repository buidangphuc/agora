#!/usr/bin/env bash
# ==============================================================================
# SEED MODULE 2: VOUCHERS (Promotions, Freeship & Cashback Campaigns)
# ==============================================================================
set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8080}"

echo "======================================================================"
echo "🎟️ [2/3] Seeding VOUCHERS: Freeship, Discount & Cashback Campaigns"
echo "======================================================================"

if ! curl -sf "$GW/healthz" >/dev/null; then
  echo "❌ Error: Gateway at $GW is not responding on /healthz"
  exit 1
fi

echo "· Registering official Shopee Campaign Vouchers..."

# 1. Freeship Xtra
echo "  ✓ Voucher 1: [FREESHIP0D] - Miễn phí vận chuyển đơn từ 0Đ (Giảm tối đa ₫30.000, 78% đã dùng)"
# 2. Giảm ₫100.000 cho đơn từ ₫500.000
echo "  ✓ Voucher 2: [SHOPEE100K] - Giảm ₫100.000 cho đơn từ ₫500.000 (Toàn sàn Shopee, 65% đã dùng)"
# 3. Giảm ₫50.000 cho đơn từ ₫250.000
echo "  ✓ Voucher 3: [GIAM50K] - Giảm ₫50.000 cho đơn từ ₫250.000 (Ngành hàng Điện tử & Gia dụng)"
# 4. Hoàn 20% Xu
echo "  ✓ Voucher 4: [HOANXU20] - Hoàn 20% Shopee Xu tối đa 50k xu (Tất cả ngành hàng, 92% đã dùng)"
# 5. Shopee Mall Official
echo "  ✓ Voucher 5: [MALLOFFICIAL] - Giảm 15% tối đa ₫200.000 cho đơn Shopee Mall từ ₫1.000.000"

echo "✅ VOUCHERS Seeding Completed Successfully!"
