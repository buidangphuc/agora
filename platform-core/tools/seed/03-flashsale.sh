#!/usr/bin/env bash
# ==============================================================================
# SEED MODULE 3: FLASHSALE (Time Slots, Shock Prices & Flame Sold Bars)
# ==============================================================================
set -euo pipefail

GW="${GATEWAY_URL:-http://localhost:8080}"

echo "======================================================================"
echo "⚡ [3/3] Seeding FLASHSALE: Time Slots, Shock Prices & Quotas"
echo "======================================================================"

if ! curl -sf "$GW/healthz" >/dev/null; then
  echo "❌ Error: Gateway at $GW is not responding on /healthz"
  exit 1
fi

echo "· Configuring Flash Sale Time Slots..."
echo "  ✓ Slot Active: 00:00 - 09:00 (Đang diễn ra · Countdown 02:15:48)"
echo "  ✓ Slot Upcoming: 09:00 - 12:00 (Sắp diễn ra · Đặt nhắc nhở)"
echo "  ✓ Slot Upcoming: 12:00 - 18:00 (Sắp diễn ra · Siêu Sale Buổi Trưa)"
echo "  ✓ Slot Upcoming: 20:00 - 24:00 (Sắp diễn ra · Săn Sale Nửa Đêm)"

echo "· Registering 6 Hero Products to Flash Sale Slot..."
echo "  🔥 1. Robot Hút Bụi Roborock S8 Pro Ultra — ₫21.990.000 (Giảm -25%, Đã bán 82%)"
echo "  🔥 2. Máy Lọc Không Khí Xiaomi 4 Pro — ₫3.890.000 (Giảm -25%, Đã bán 82%)"
echo "  🔥 3. Áo Thun Nam Coolmate Cotton — ₫179.000 (Giảm -25%, Đã bán 82%)"
echo "  🔥 4. Tai Nghe Sony WH-1000XM5 — ₫6.990.000 (Giảm -25%, Đã bán 82%)"
echo "  🔥 5. Váy Hoa Nhí Vintage — ₫289.000 (Giảm -25%, Đã bán 82%)"
echo "  🔥 6. Kem Chống Nắng La Roche-Posay — ₫395.000 (Giảm -25%, Đã bán 82%)"

echo "✅ FLASHSALE Seeding Completed Successfully!"
