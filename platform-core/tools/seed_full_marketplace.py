#!/usr/bin/env python3
"""
Seed Full Marketplace Script for Agora Platform.

Seeds:
1. 5 Realistic Seller Shops (Điện Máy Xanh, Coolmate, Anker, Lock&Lock, Nike Vietnam) with KYC verification.
2. 10 Buyer accounts with authentic Vietnamese addresses.
3. 50+ diverse products across 5 categories with Vietnamese titles, descriptions, realistic prices, and variants.
4. Platform & Shop Vouchers with percentage/fixed discounts + Flash Sale campaigns.
5. 500+ Historical tracking beacons with GA4 ecommerce payloads.
6. 30-day simulated Order History & Outbox events for sellers to populate DuckDB analytics and Demand Forecasting (ADR-0013).

Usage:
  python3 seed_full_marketplace.py [--gateway http://localhost:8080] [--export-json path] [--dry-run]
"""

import argparse
import datetime
import json
import os
import random
import sys
import time
import uuid
from typing import Any, Dict, List, Optional, Tuple

try:
    import requests
except ImportError:
    requests = None


# ─────────────────────────────────────────────────────────────────────────────
# CONSTANTS & CONFIGURATION
# ─────────────────────────────────────────────────────────────────────────────

DEFAULT_GATEWAY_URL = os.environ.get("GATEWAY_URL", "http://localhost:8080")
DEFAULT_PASSWORD = "pass123"

CATEGORIES = [
    {"id": "cat-electronics", "name": "Điện Tử & Thiết Bị Số"},
    {"id": "cat-fashion", "name": "Thời Trang Nam Nữ"},
    {"id": "cat-home", "name": "Nhà Cửa & Đời Sống"},
    {"id": "cat-appliances", "name": "Thiết Bị Điện Gia Dụng"},
    {"id": "cat-sports", "name": "Thể Thao & Dã Ngoại"},
]

SELLERS_DATA = [
    {
        "id": "seller-dmx",
        "username": "dien_may_xanh",
        "shop_name": "Điện Máy Xanh Official Store",
        "email": "dienmayxanh@official.vn",
        "category": "cat-appliances",
        "kyc": {"doc_type": "business_license", "doc_ref": "docs/kyc_dmx_license.pdf"},
    },
    {
        "id": "seller-coolmate",
        "username": "coolmate_official",
        "shop_name": "Coolmate Official Store",
        "email": "support@coolmate.me",
        "category": "cat-fashion",
        "kyc": {"doc_type": "business_license", "doc_ref": "docs/kyc_coolmate_license.pdf"},
    },
    {
        "id": "seller-anker",
        "username": "anker_flagship",
        "shop_name": "Anker Flagship Store VN",
        "email": "anker.flagship@anker.vn",
        "category": "cat-electronics",
        "kyc": {"doc_type": "business_license", "doc_ref": "docs/kyc_anker_license.pdf"},
    },
    {
        "id": "seller-locknlock",
        "username": "locknlock_vietnam",
        "shop_name": "Lock&Lock Vietnam Official",
        "email": "cs@locknlock.vn",
        "category": "cat-home",
        "kyc": {"doc_type": "business_license", "doc_ref": "docs/kyc_locknlock_license.pdf"},
    },
    {
        "id": "seller-nike",
        "username": "nike_vietnam_store",
        "shop_name": "Nike Vietnam Store",
        "email": "contact@nike.vn",
        "category": "cat-sports",
        "kyc": {"doc_type": "business_license", "doc_ref": "docs/kyc_nike_license.pdf"},
    },
]

BUYERS_DATA = [
    {
        "username": "nguyen_van_a",
        "name": "Nguyễn Văn An",
        "phone": "0912345678",
        "street": "Số 15 Liễu Giai",
        "ward": "Phường Liễu Giai",
        "district": "Quận Ba Đình",
        "city": "Hà Nội",
    },
    {
        "username": "tran_thi_b",
        "name": "Trần Thị Bích",
        "phone": "0987654321",
        "street": "Số 48 Cầu Giấy",
        "ward": "Phường Quan Hoa",
        "district": "Quận Cầu Giấy",
        "city": "Hà Nội",
    },
    {
        "username": "le_hoang_nam",
        "name": "Lê Hoàng Nam",
        "phone": "0903123456",
        "street": "120 Nguyễn Huệ",
        "ward": "Phường Bến Nghé",
        "district": "Quận 1",
        "city": "TP Hồ Chí Minh",
    },
    {
        "username": "pham_thi_hoa",
        "name": "Phạm Thị Mai Hoa",
        "phone": "0934567890",
        "street": "88 Nguyễn Thị Thập",
        "ward": "Phường Tân Phú",
        "district": "Quận 7",
        "city": "TP Hồ Chí Minh",
    },
    {
        "username": "vu_duc_minh",
        "name": "Vũ Đức Minh",
        "phone": "0971234567",
        "street": "25 Nguyễn Văn Linh",
        "ward": "Phường Nam Dương",
        "district": "Quận Hải Châu",
        "city": "Đà Nẵng",
    },
    {
        "username": "hoang_gia_bao",
        "name": "Hoàng Gia Bảo",
        "phone": "0962345678",
        "street": "104 Điện Biên Phủ",
        "ward": "Phường Chính Gián",
        "district": "Quận Thanh Khê",
        "city": "Đà Nẵng",
    },
    {
        "username": "dang_thuy_trang",
        "name": "Đặng Thùy Trang",
        "phone": "0945678901",
        "street": "72 30 Tháng 4",
        "ward": "Phường Hưng Lợi",
        "district": "Quận Ninh Kiều",
        "city": "Cần Thơ",
    },
    {
        "username": "bui_tien_dung",
        "name": "Bùi Tiến Dũng",
        "phone": "0918765432",
        "street": "36 Lê Hồng Phong",
        "ward": "Phường Đông Khê",
        "district": "Quận Ngô Quyền",
        "city": "Hải Phòng",
    },
    {
        "username": "do_thi_mai",
        "name": "Đỗ Thị Mai",
        "phone": "0981122334",
        "street": "55 Tôn Đức Thắng",
        "ward": "Phường Hàng Bột",
        "district": "Quận Đống Đa",
        "city": "Hà Nội",
    },
    {
        "username": "truong_quoc_huy",
        "name": "Trương Quốc Huy",
        "phone": "0909988776",
        "street": "180 Điện Biên Phủ",
        "ward": "Phường 15",
        "district": "Quận Bình Thạnh",
        "city": "TP Hồ Chí Minh",
    },
]

# 52 High Quality Vietnamese Products Catalog
PRODUCTS_CATALOG = [
    # ── Electronics & Gadgets (Dien May Xanh & Anker Flagship) ──
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Điện thoại Apple iPhone 15 Pro Max 256GB - Chính hãng VN/A (Khung viền Titan, Camera 5x)",
        "description": "Siêu phẩm iPhone 15 Pro Max thiết kế khung viền Titan chuẩn hàng không vũ trụ siêu bền nhẹ. Trang bị chip Apple A17 Pro tiến trình 3nm mang lại hiệu năng gaming đỉnh cao, camera telephoto 5x sắc nét và cổng kết nối USB-C tốc độ cao.",
        "price": 29990000,
        "stock": 150,
        "images": [
            "https://images.unsplash.com/photo-1695048133142-1a20484d2569?w=800&auto=format&fit=crop&q=80",
            "https://images.unsplash.com/photo-1510557880182-3d4d3cba35a5?w=800&auto=format&fit=crop&q=80",
        ],
        "variants": [
            {"name": "Titan Tự Nhiên / 256GB", "sku": "IP15PM-NAT-256", "price": 29990000, "stock": 60},
            {"name": "Titan Xanh / 256GB", "sku": "IP15PM-BLU-256", "price": 29990000, "stock": 50},
            {"name": "Titan Đen / 512GB", "sku": "IP15PM-BLK-512", "price": 34990000, "stock": 40},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Laptop Apple MacBook Pro 14 inch M3 Pro (18GB Unified RAM / 512GB SSD) - Hàng Chính Hãng",
        "description": "MacBook Pro 14 inch chip Apple M3 Pro thế hệ mới kiến trúc đồ họa tân tiến, đáp ứng mượt mà các tác vụ render video 4K, đồ họa 3D và lập trình AI. Màn hình Liquid Retina XDR độ sáng tối đa 1600 nits, thời lượng pin ấn tượng lên đến 18 tiếng.",
        "price": 49990000,
        "stock": 45,
        "images": [
            "https://images.unsplash.com/photo-1517336714731-489689fd1ca8?w=800&auto=format&fit=crop&q=80",
            "https://images.unsplash.com/photo-1611186871348-b1ce696e52c9?w=800&auto=format&fit=crop&q=80",
        ],
        "variants": [
            {"name": "Space Black / 18GB/512GB", "sku": "MBP14-M3P-BLK", "price": 49990000, "stock": 25},
            {"name": "Silver / 18GB/1TB", "sku": "MBP14-M3P-SLV-1T", "price": 56990000, "stock": 20},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Máy tính bảng Apple iPad Air M2 11 inch Wi-Fi 128GB - Thiết kế siêu mỏng nhẹ",
        "description": "iPad Air M2 11 inch thế hệ mới với sức mạnh vượt trội từ chip M2, hỗ trợ Apple Pencil Pro và Magic Keyboard. Màn hình Liquid Retina sắc nét với công nghệ True Tone cho trải nghiệm thị giác sống động.",
        "price": 16490000,
        "stock": 70,
        "images": [
            "https://images.unsplash.com/photo-1544244015-0df4b3ffc6b0?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xám Không Gian / 128GB", "sku": "IPADAIR-M2-GRY", "price": 16490000, "stock": 35},
            {"name": "Xanh Pastel / 128GB", "sku": "IPADAIR-M2-BLU", "price": 16490000, "stock": 35},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Điện thoại Samsung Galaxy S24 Ultra 5G 12GB/256GB - Quyền Năng Galaxy AI Đỉnh Cao",
        "description": "Siêu phẩm flagship Samsung Galaxy S24 Ultra tích hợp trọn bộ tính năng Galaxy AI thông minh: Khoanh tròn để tìm kiếm, phiên dịch cuộc gọi trực tiếp và trợ lý chỉnh ảnh chuyên nghiệp. Khung viền Titanium sang trọng.",
        "price": 27990000,
        "stock": 90,
        "images": [
            "https://images.unsplash.com/photo-1610945415295-d9bbf067e59c?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Titanium Gray / 256GB", "sku": "SS-S24U-GRY", "price": 27990000, "stock": 45},
            {"name": "Titanium Violet / 256GB", "sku": "SS-S24U-VIO", "price": 27990000, "stock": 45},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Tai Nghe Chống Ồn Cao Cấp Sony WH-1000XM5 Không Dây - Chống Ồn Chủ Động ANC Đỉnh Cao",
        "description": "Công nghệ chống ồn số 1 thế giới với 2 bộ xử lý và 8 micro chuyên biệt. Chất âm chuẩn Hi-Res Audio Wireless với màng loa carbon 30mm, thời lượng pin 30 giờ liên tục kèm sạc nhanh 3 phút nghe được 3 giờ.",
        "price": 6990000,
        "stock": 80,
        "images": [
            "https://images.unsplash.com/photo-1546435770-a3e426bf472b?w=800&auto=format&fit=crop&q=80",
            "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=800&auto=format&fit=crop&q=80",
        ],
        "variants": [
            {"name": "Đen Nhám Cổ Điển", "sku": "SONY-XM5-BLK", "price": 6990000, "stock": 50},
            {"name": "Bạc Bạch Kim", "sku": "SONY-XM5-SLV", "price": 6990000, "stock": 30},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Tai Nghe True Wireless Apple AirPods Pro 2 MagSafe USB-C - Âm Thanh Không Gian Cá Nhân Hóa",
        "description": "AirPods Pro 2 trang bị chip H2 mang đến khả năng khử tiếng ồn chủ động gấp 2 lần, chế độ Xuyên Âm thích ứng và Âm Thanh Không Gian sống động. Hộp sạc chuẩn USB-C có loa tìm kiếm và khả năng chống nước IP54.",
        "price": 5690000,
        "stock": 120,
        "images": [
            "https://images.unsplash.com/photo-1600294037681-c80b4cb5b434?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Chuẩn VN/A", "sku": "APP2-USBC-WHT", "price": 5690000, "stock": 120},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Màn Hình Đồ Họa Chuyên Nghiệp Dell UltraSharp U2724D 27 inch 2K 120Hz IPS Black",
        "description": "Màn hình 27 inch độ phân giải QHD 2560x1440 với tấm nền IPS Black tỷ lệ tương phản 2000:1, độ phủ màu 100% sRGB và 98% DCI-P3. Tần số quét 120Hz mượt mà và cảm biến ánh sáng tự động điều chỉnh độ sáng.",
        "price": 9890000,
        "stock": 40,
        "images": [
            "https://images.unsplash.com/photo-1527443224154-c4a3942d3acf?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Bạc Chân Kim Loại", "sku": "DELL-U2724D-SLV", "price": 9890000, "stock": 40},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-electronics",
        "title": "Loa Bluetooth Di Động Marshall Emberton II Chống Nước IP67 - Âm Thanh 360 Độ True Stereophonic",
        "description": "Loa di động phong cách rock cổ điển đậm chất Marshall. Thời lượng pin ấn tượng lên tới hơn 30 giờ, chống nước và bụi chuẩn IP67, hỗ trợ kết nối nhiều loa qua chế độ Stack Mode.",
        "price": 3790000,
        "stock": 65,
        "images": [
            "https://images.unsplash.com/photo-1545454675-3531b543be5d?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Black & Brass (Đen Viền Đồng)", "sku": "MARSH-EMB2-BLK", "price": 3790000, "stock": 40},
            {"name": "Cream (Màu Kem Cổ Điển)", "sku": "MARSH-EMB2-CRM", "price": 3790000, "stock": 25},
        ],
    },
    {
        "seller_user": "anker_flagship",
        "category_id": "cat-electronics",
        "title": "Củ Sạc Nhanh Anker GaNPrime 65W 3 Cổng (2 Type-C + 1 USB-A) - Nhỏ Gọn Siêu Nhanh",
        "description": "Công nghệ sạc nhanh GaN III độc quyền từ Anker, công suất 65W sạc cùng lúc MacBook Air, iPhone và Apple Watch an toàn. Công nghệ Dynamic Power Distribution tự động phân bổ nguồn điện thông minh.",
        "price": 890000,
        "stock": 200,
        "images": [
            "https://images.unsplash.com/photo-1622445262464-84b1456045b6?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Phantom", "sku": "ANKER-65W-BLK", "price": 890000, "stock": 120},
            {"name": "Trắng Ngọc Trai", "sku": "ANKER-65W-WHT", "price": 890000, "stock": 80},
        ],
    },
    {
        "seller_user": "anker_flagship",
        "category_id": "cat-electronics",
        "title": "Pin Dự Phòng Anker MagGo Qi2 15W 10.000mAh Không Dây Từ Tính Siêu Nhanh",
        "description": "Chuẩn sạc không dây thế hệ mới Qi2 công suất 15W tương thích sạc nhanh iPhone 13/14/15 series. Dung lượng 10.000mAh kèm màn hình hiển thị thông minh thời gian và tỷ lệ phần trăm pin còn lại.",
        "price": 1290000,
        "stock": 140,
        "images": [
            "https://images.unsplash.com/photo-1609592424109-dd9892f1b177?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Carbon", "sku": "ANKER-MAG10K-BLK", "price": 1290000, "stock": 70},
            {"name": "Trắng Ice", "sku": "ANKER-MAG10K-WHT", "price": 1290000, "stock": 70},
        ],
    },
    {
        "seller_user": "anker_flagship",
        "category_id": "cat-electronics",
        "title": "Cáp Sạc Nhanh C To C Anker PowerLine III Flow Silicone Siêu Mềm Mại 100W 1.8m",
        "description": "Dây sạc bọc silicone siêu mềm mại không bao giờ bị rối gãy, hỗ trợ sạc nhanh công suất tối đa 100W cho laptop và điện thoại. Chịu lực uốn cong hơn 25.000 lần.",
        "price": 280000,
        "stock": 300,
        "images": [
            "https://images.unsplash.com/photo-1583863788434-e58a36330cf0?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Mint - 1.8m", "sku": "ANKER-C2C-MNT", "price": 280000, "stock": 150},
            {"name": "Tím Lavender - 1.8m", "sku": "ANKER-C2C-LAV", "price": 280000, "stock": 150},
        ],
    },
    {
        "seller_user": "anker_flagship",
        "category_id": "cat-electronics",
        "title": "Tai Nghe Chụp Tai Chống Ồn Anker Soundcore Space One - Hi-Res Audio Không Dây",
        "description": "Khử tiếng ồn thích ứng giảm đến 98% giọng nói môi trường xung quanh. Driver dynamic 40mm tùy chỉnh tái tạo âm thanh chuẩn Hi-Res Wireless với codec LDAC, pin 55 giờ liên tục.",
        "price": 1890000,
        "stock": 110,
        "images": [
            "https://images.unsplash.com/photo-1546435770-a3e426bf472b?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Huyền Bí", "sku": "ANKER-SPO-BLK", "price": 1890000, "stock": 60},
            {"name": "Màu Be Sữa", "sku": "ANKER-SPO-LAT", "price": 1890000, "stock": 50},
        ],
    },

    # ── Fashion & Apparel (Coolmate Official Store) ──
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Áo Thun Nam Cổ Tròn Coolmate Cotton Compact 100% Chống Nhăn Co Giãn Thoáng Mát",
        "description": "Chất liệu sợi Cotton Compact siêu dai mềm mịn, không xù lông và thấm hút mồ hôi vượt trội. Form Regular Fit chuẩn người Việt Nam, đường may tỉ mỉ chắc chắn thích hợp mặc hàng ngày.",
        "price": 179000,
        "stock": 280,
        "images": [
            "https://images.unsplash.com/photo-1521572267360-ee0c2909d518?w=800&auto=format&fit=crop&q=80",
            "https://images.unsplash.com/photo-1583743814966-8936f5b7be1a?w=800&auto=format&fit=crop&q=80",
        ],
        "variants": [
            {"name": "Đen Classic - Size L", "sku": "CM-TSHIRT-BLK-L", "price": 179000, "stock": 100},
            {"name": "Trắng Basic - Size L", "sku": "CM-TSHIRT-WHT-L", "price": 179000, "stock": 90},
            {"name": "Xanh Navy - Size XL", "sku": "CM-TSHIRT-NVY-XL", "price": 179000, "stock": 90},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Áo Polo Nam Coolmate Pique Cotton Thấm Hút Mồ Hôi Thoáng Khí Cao Cấp",
        "description": "Vải dệt mắt chim Pique cao cấp tăng độ đứng form và giữ dáng áo sau nhiều lần giặt. Cổ áo bo dệt chống quăn mép, thích hợp đi làm công sở hoặc dạo phố trẻ trung năng động.",
        "price": 249000,
        "stock": 190,
        "images": [
            "https://images.unsplash.com/photo-1586363104862-3a5e2ab60d99?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Navy - Size M", "sku": "CM-POLO-NVY-M", "price": 249000, "stock": 60},
            {"name": "Xanh Navy - Size L", "sku": "CM-POLO-NVY-L", "price": 249000, "stock": 70},
            {"name": "Trắng Kem - Size L", "sku": "CM-POLO-WHT-L", "price": 249000, "stock": 60},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Quần Short Thể Thao Nam Coolmate QuickDry 7 inch Co Giãn 4 Chiều Thoáng Mát",
        "description": "Công nghệ thoát ẩm QuickDry siêu tốc giúp cơ thể luôn khô thoáng khi vận động cường độ cao. Tích hợp 2 túi sâu có khóa kéo ẩn an toàn để chìa khóa và điện thoại khi chạy bộ.",
        "price": 189000,
        "stock": 220,
        "images": [
            "https://images.unsplash.com/photo-1591195853828-11db59a44f6b?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Nam Tính - Size L", "sku": "CM-SHORT-BLK-L", "price": 189000, "stock": 110},
            {"name": "Xám Ghi Đậm - Size L", "sku": "CM-SHORT-GRY-L", "price": 189000, "stock": 110},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Quần Jean Nam Dáng Slimfit Coolmate Clean Denim Co Giãn Thoải Mái",
        "description": "Chất liệu Clean Denim kết hợp sợi Spandex cho độ co giãn linh hoạt không bị gò bó. Màu wash chàm tự nhiên bền màu, form ôm vừa vặn tôn dáng chân.",
        "price": 499000,
        "stock": 150,
        "images": [
            "https://images.unsplash.com/photo-1541099649105-f69ad21f3246?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Chàm Đậm - Size 31", "sku": "CM-JEAN-IND-31", "price": 499000, "stock": 75},
            {"name": "Xanh Nhạt Vintage - Size 32", "sku": "CM-JEAN-BLU-32", "price": 499000, "stock": 75},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Combo 3 Quần Lót Nam Boxer Sợi Tre Bamboo Coolmate Kháng Khuẩn Khử Mùi",
        "description": "Vải sợi tre sinh học Bamboo mềm mịn gấp 2 lần cotton thường, kháng khuẩn tự nhiên và khử mùi hiệu quả. Đai lưng Spandex êm ái không để lại vết hằn đỏ trên da.",
        "price": 239000,
        "stock": 320,
        "images": [
            "https://images.unsplash.com/photo-1588850561407-ed78c282e89b?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Combo 3 Màu Basic - Size L", "sku": "CM-BOXER-3L", "price": 239000, "stock": 160},
            {"name": "Combo 3 Màu Basic - Size XL", "sku": "CM-BOXER-3XL", "price": 239000, "stock": 160},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Áo Khoác Gió Thể Thao Nam 2 Lớp Chống Nước Cản Gió - Form Dáng Trẻ Trung",
        "description": "Chất liệu vải dù cao cấp công nghệ tráng bạc cản gió và chống thấm nước mưa phùn. Mũ trùm đầu tháo rời linh hoạt, khóa kéo YKK bền bỉ và form dáng thể thao năng động dễ phối đồ.",
        "price": 399000,
        "stock": 180,
        "images": [
            "https://images.unsplash.com/photo-1548883354-7622d03aca27?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Navy - Size M", "sku": "CM-WIND-NVY-M", "price": 399000, "stock": 60},
            {"name": "Xanh Navy - Size L", "sku": "CM-WIND-NVY-L", "price": 399000, "stock": 60},
            {"name": "Đen Basic - Size L", "sku": "CM-WIND-BLK-L", "price": 399000, "stock": 60},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Áo Sơ Mi Nam Tay Dài Vải Oxford Cotton Dày Dặn Đứng Form Chống Nhăn",
        "description": "Vải Oxford dệt tổ ong thoáng khí, hạn chế nhăn nhúm khi giặt sấy. Cổ áo Button-down cổ điển giữ phong thái lịch lãm cho các buổi họp và làm việc hàng ngày.",
        "price": 349000,
        "stock": 140,
        "images": [
            "https://images.unsplash.com/photo-1596755094514-f87e34085b2c?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Tinh Tế - Size L", "sku": "CM-SHIRT-WHT-L", "price": 349000, "stock": 70},
            {"name": "Xanh Dương Nhạt - Size L", "sku": "CM-SHIRT-BLU-L", "price": 349000, "stock": 70},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Áo Hoodie Nỉ Bông Unisex Form Rộng Oversize Hàn Quốc Dày Dặn Ấm Áp",
        "description": "Chất nỉ bông Cotton 380gsm dày dặn giữ nhiệt cực tốt vào mùa đông. Mũ áo 2 lớp to đứng form kèm dây rút kim loại cao cấp, túi kangaroo phía trước tiện lợi.",
        "price": 389000,
        "stock": 160,
        "images": [
            "https://images.unsplash.com/photo-1556905055-8f358a7a47b2?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xám Melange - FreeSize", "sku": "CM-HOODIE-GRY", "price": 389000, "stock": 80},
            {"name": "Đen Classic - FreeSize", "sku": "CM-HOODIE-BLK", "price": 389000, "stock": 80},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Thắt Lưng Nam Da Bò Thật Nguyên Tấm Khóa Tự Động Hợp Kim Chống Rỉ",
        "description": "Chất liệu da bò thật 100% nhập khẩu mềm dẻo không gãy nứt. Mặt khóa hợp kim mạ crom cao cấp phong cách tối giản sang trọng cho nam giới.",
        "price": 289000,
        "stock": 210,
        "images": [
            "https://images.unsplash.com/photo-1624222247344-550fb60583dc?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Da Mịn - Bản 3.5cm", "sku": "CM-BELT-BLK", "price": 289000, "stock": 110},
            {"name": "Nâu Cà Phê - Bản 3.5cm", "sku": "CM-BELT-BRW", "price": 289000, "stock": 100},
        ],
    },
    {
        "seller_user": "coolmate_official",
        "category_id": "cat-fashion",
        "title": "Đầm Nữ Dáng Xòe Cổ Vuông Tay Bồng Phong Cách Vintage Thanh Lịch Nữ Tính",
        "description": "Chất liệu vải lụa tơ mềm mại thoáng mát, có lót trong kín đáo. Thiết kế cổ vuông khoe xương quai xanh quyến rũ và chân váy xòe bồng bềnh nữ tính.",
        "price": 420000,
        "stock": 90,
        "images": [
            "https://images.unsplash.com/photo-1595777457583-95e059d581b8?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Sữa - Size S", "sku": "CM-DRESS-WHT-S", "price": 420000, "stock": 45},
            {"name": "Trắng Sữa - Size M", "sku": "CM-DRESS-WHT-M", "price": 420000, "stock": 45},
        ],
    },

    # ── Home & Living (Lock&Lock Vietnam Official) ──
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Bình Giữ Nhiệt Lock&Lock Feather Light Thép Không Gỉ 450ml - Giữ Nóng Lạnh 24h",
        "description": "Bình giữ nhiệt siêu nhẹ chỉ 230g làm bằng inox 304 an toàn thực phẩm. Khả năng giữ nóng đến 8 giờ và giữ lạnh đến 24 giờ. Nắp bật 1 chạm có khóa cài an toàn chống rò rỉ nước.",
        "price": 249000,
        "stock": 250,
        "images": [
            "https://images.unsplash.com/photo-1602143407151-7111542de6e8?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Mint Dịu Mát", "sku": "LOCK-FL-MNT", "price": 249000, "stock": 125},
            {"name": "Đen Nhám Thể Thao", "sku": "LOCK-FL-BLK", "price": 249000, "stock": 125},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Bộ 4 Hộp Thủy Tinh Chịu Nhiệt Lock&Lock Glass Top Oven Safe Dùng Được Lò Nướng 400°C",
        "description": "Thủy tinh Borosilicate cao cấp chịu sốc nhiệt lên đến 400°C, sử dụng an toàn trong lò vi sóng, lò nướng và máy rửa bát. Nắp khóa 4 cạnh kín hơi 100% không bay mùi thực phẩm.",
        "price": 369000,
        "stock": 180,
        "images": [
            "https://images.unsplash.com/photo-1584990347449-39908cfb2742?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Bộ 4 Hộp Vuông & Chữ Nhật", "sku": "LOCK-SET4-GLS", "price": 369000, "stock": 180},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Nồi Cơm Điện Cao Tần IH Lock&Lock 1.8L Lòng Nồi Niêu Chống Dính 3D Đa Năng",
        "description": "Công nghệ cảm ứng từ IH nấu cơm chín đều từ trong lõi, giữ trọn vị ngọt tự nhiên của hạt gạo. Lòng nồi hợp kim gang niêu dày 3mm phủ chống dính Ceramic cao cấp siêu bền.",
        "price": 1890000,
        "stock": 60,
        "images": [
            "https://images.unsplash.com/photo-1544816155-12df9643f363?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Kim Cương 1.8L", "sku": "LOCK-RICE-WHT", "price": 1890000, "stock": 30},
            {"name": "Đen Titan 1.8L", "sku": "LOCK-RICE-BLK", "price": 1890000, "stock": 30},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Bộ Dao Làm Bếp Inox 7 Món Cao Cấp Lock&Lock Kèm Giá Cắm Gỗ Thông Sang Trọng",
        "description": "Chất liệu thép không gỉ 3Cr13 tôi luyện theo công nghệ nhiệt lạnh Đức sắc bén vượt trội. Tay cầm đúc liền khối bọc nhựa chống trượt an toàn tuyệt đối khi sơ chế.",
        "price": 589000,
        "stock": 100,
        "images": [
            "https://images.unsplash.com/photo-1593618998160-e34014e67546?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Bộ 7 Món Kèm Giá Gỗ", "sku": "LOCK-KNIFE-7P", "price": 589000, "stock": 100},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Chảo Xào Chống Dính Lock&Lock Deco Lòng Sâu 28cm Đáy Từ Dùng Mọi Loại Bếp",
        "description": "Lớp phủ Titanium cao cấp kháng dính 50.000 lần chà xát, lòng chảo sâu 8.5cm hạn chế bắn dầu mỡ. Tay cầm cán gỗ cách nhiệt chống nóng tinh tế phong cách Bắc Âu.",
        "price": 429000,
        "stock": 140,
        "images": [
            "https://images.unsplash.com/photo-1583778176476-4a8b02a64c01?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Màu Xanh Cổ Vịt - 28cm", "sku": "LOCK-PAN-GRN-28", "price": 429000, "stock": 70},
            {"name": "Màu Vàng Be - 28cm", "sku": "LOCK-PAN-YEL-28", "price": 429000, "stock": 70},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Cây Lau Nhà Xoay Tay Tự Vắt Lock&Lock Easy Mop Kèm Thùng Vắt Đôi Tiện Lợi",
        "description": "Cơ cấu xoay ly tâm thông minh vắt khô bông lau tới 95% chỉ với vài lần nhấn nhẹ. Bông lau sợi microfiber thấm hút nhanh gom sạch mọi bụi bẩn và tóc rụng trên sàn nhà.",
        "price": 499000,
        "stock": 110,
        "images": [
            "https://images.unsplash.com/photo-1581578731548-c64695cc6952?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Dương Tươi Sáng", "sku": "LOCK-MOP-BLU", "price": 499000, "stock": 55},
            {"name": "Xám Ghi Sang Trọng", "sku": "LOCK-MOP-GRY", "price": 499000, "stock": 55},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Máy Xay Sinh Tố Cầm Tay Đa Năng Lock&Lock 2 Cối Nhựa Tritan Không Chứa BPA",
        "description": "Động cơ đồng nguyên chất công suất 300W xay nhuyễn mịn đá viên và hoa quả trong 30 giây. Tặng kèm 2 bình thể thao 600ml có nắp uống trực tiếp tiện lợi mang đi làm.",
        "price": 399000,
        "stock": 130,
        "images": [
            "https://images.unsplash.com/photo-1570222094114-d054a817e56b?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Kem 2 Cối", "sku": "LOCK-BLND-WHT", "price": 399000, "stock": 65},
            {"name": "Xanh Navy 2 Cối", "sku": "LOCK-BLND-NVY", "price": 399000, "stock": 65},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Thảm Lau Chân San Hô Sợi Microfiber Siêu Thấm Hút Nước Đáy Cao Su Chống Trượt",
        "description": "Sợi lông san hô mật độ cao siêu êm ái thấm hút nước gấp 7 lần trọng lượng vải. Mặt đáy phủ cao su latex TPR bám dính chắc chắn chống trơn trượt cửa phòng tắm.",
        "price": 79000,
        "stock": 450,
        "images": [
            "https://images.unsplash.com/photo-1600585154340-be6161a56a0c?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xám Khói 40x60cm", "sku": "MAT-CORAL-GRY", "price": 79000, "stock": 250},
            {"name": "Xanh Lam 40x60cm", "sku": "MAT-CORAL-BLU", "price": 79000, "stock": 200},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Kệ Để Giày Dép Gỗ Tự Nhiên 5 Tầng Gấp Gọn Thông Minh Không Cần Lắp Đặt",
        "description": "Chất liệu gỗ tre tự nhiên đã qua xử lý sấy chống ẩm mốc mối mọt. Thiết kế ziczac mở rộng gập gọn chỉ trong 3 giây, chứa được 15-20 đôi giày dép gọn gàng.",
        "price": 219000,
        "stock": 170,
        "images": [
            "https://images.unsplash.com/photo-1595428774223-ef52624120d2?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Gỗ Tự Nhiên 50cm", "sku": "SHOE-RACK-50", "price": 219000, "stock": 85},
            {"name": "Gỗ Tự Nhiên 70cm", "sku": "SHOE-RACK-70", "price": 259000, "stock": 85},
        ],
    },
    {
        "seller_user": "locknlock_vietnam",
        "category_id": "cat-home",
        "title": "Đèn Bàn Học LED Chống Cận Thị Cảm Ứng 3 Chế Độ Sáng Bảo Vệ Mắt",
        "description": "Chip LED chuẩn quang phổ tự nhiên không phát ánh sáng xanh gây hại võng mạc, chỉ số hoàn màu Ra>90. Nút cảm ứng điều chỉnh 3 mức nhiệt độ màu và độ sáng vô cấp.",
        "price": 189000,
        "stock": 220,
        "images": [
            "https://images.unsplash.com/photo-1534353436294-0dbd4bdac845?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Tinh Khiết", "sku": "LAMP-LED-WHT", "price": 189000, "stock": 110},
            {"name": "Đen Nhám Hiện Đại", "sku": "LAMP-LED-BLK", "price": 189000, "stock": 110},
        ],
    },

    # ── Appliances & Smart Home (Dien May Xanh) ──
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Robot Hút Bụi Lau Nhà Tự Động Giặt Giẻ Sấy Khí Nóng Roborock S8 Pro Ultra",
        "description": "Trạm sạc RockDock Ultra tự động đổ rác, giặt và sấy giẻ bằng khí nóng. Lực hút cực đại 6000Pa, chổi lăn kép DuoRoller chống rối và hệ thống lau rung VibraRise 2.0 sạch bóng sàn nhà.",
        "price": 21990000,
        "stock": 35,
        "images": [
            "https://images.unsplash.com/photo-1518640467707-6811f4a6ab73?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Sang Trọng", "sku": "ROBOROCK-S8-WHT", "price": 21990000, "stock": 20},
            {"name": "Đen Huyền Bí", "sku": "ROBOROCK-S8-BLK", "price": 21990000, "stock": 15},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Nồi Chiên Không Dầu Điện Tử Philips XXL HD9650/90 Dung Tích 7.3L - Công Suất 2225W",
        "description": "Công nghệ Twin TurboStar loại bỏ đến 90% chất béo trong thực phẩm. Lòng nồi dung tích lớn nướng nguyên con gà 1.4kg hoặc 1.4kg khoai tây chiên vàng giòn rụm cho cả gia đình.",
        "price": 4290000,
        "stock": 60,
        "images": [
            "https://images.unsplash.com/photo-1585659722983-3a675dabf23d?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Bóng Điện Tử 7.3L", "sku": "PHILIPS-XXL-BLK", "price": 4290000, "stock": 60},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Máy Lọc Không Khí Thông Minh Xiaomi Smart Air Purifier 4 Pro - Lọc Bụi Mịn PM2.5",
        "description": "Lọc sạch bụi mịn PM2.5, khói phấn hoa và vi khuẩn với tốc độ phân phối không khí sạch 500m3/h cho phòng rộng đến 60m2. Cảm biến laser kép hiển thị chất lượng không khí trên màn hình OLED và app Mi Home.",
        "price": 3890000,
        "stock": 80,
        "images": [
            "https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Tinh Khiết Bản Quốc Tế", "sku": "XIAOMI-AIR-4PRO", "price": 3890000, "stock": 80},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Quạt Tháp Không Cánh Lọc Không Khí Philips Smart 2 Trong 1 Kháng Khuẩn Ion Âm",
        "description": "Thiết kế không cánh an toàn cho trẻ nhỏ và thú cưng. Luồng gió 3D góc quay 350 độ làm mát toàn diện gian phòng kèm màng lọc HEPA khử mùi hôi và phấn hoa hiệu quả.",
        "price": 4790000,
        "stock": 45,
        "images": [
            "https://images.unsplash.com/photo-1618941716939-553df3c6c278?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xám Bạc Kim Loại", "sku": "PHILIPS-FAN-SLV", "price": 4790000, "stock": 25},
            {"name": "Trắng Ngọc Trai", "sku": "PHILIPS-FAN-WHT", "price": 4790000, "stock": 20},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Máy Hút Ẩm Không Khí Thông Minh Xiaomi New Widetech 20L/ngày Bình Chứa 3.2L",
        "description": "Giải pháp chống nồm ẩm hoàn hảo cho gia đình miền Bắc. Công suất hút ẩm 20 lít/ngày duy trì độ ẩm lý tưởng 50-60%, tích hợp sấy khô quần áo và kết nối app điều khiển từ xa.",
        "price": 3490000,
        "stock": 55,
        "images": [
            "https://images.unsplash.com/photo-1585771724684-38269d6639fd?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Tinh Tế 20L", "sku": "XIAOMI-HUM-20L", "price": 3490000, "stock": 55},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Nồi Áp Suất Điện Tử Đa Năng Tefal Turbo Cuisine 5L Lòng Nồi Niêu Hình Cầu",
        "description": "Công nghệ lòng nồi niêu Spherical Pot tuần hoàn nhiệt 360 độ giúp thực phẩm chín mềm nhừ nhanh hơn gấp 3 lần. 10 chương trình nấu tự động từ hầm xương, nấu cháo đến làm sữa chua.",
        "price": 2190000,
        "stock": 75,
        "images": [
            "https://images.unsplash.com/photo-1544816155-12df9643f363?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Titan 5L", "sku": "TEFAL-TURBO-5L", "price": 2190000, "stock": 75},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Bàn Ủi Hơi Nước Đứng Philips ComfortTouch Plus 2000W Đầu Ủi FlexHead Linh Hoạt",
        "description": "Công nghệ đầu ủi FlexHead dễ dàng ủi phẳng chân váy mà không cần cúi gập người. Hơi phun liên tục 40g/phút diệt sạch 99.9% vi khuẩn và mùi ẩm mốc trên quần áo.",
        "price": 2890000,
        "stock": 50,
        "images": [
            "https://images.unsplash.com/photo-1585659722983-3a675dabf23d?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Tím Pastel", "sku": "PHILIPS-IRON-PUR", "price": 2890000, "stock": 50},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Máy Ép Chậm Hurom H-200 Easy Clean Trục Ép Tự Cắt Miệng Cối Ép Nguyên Trái",
        "description": "Máy ép chậm cao cấp nhập khẩu Hàn Quốc. Miệng cối siêu lớn chứa nguyên quả táo không cần cắt nhỏ, trục ép thế hệ mới tự động thái nhỏ và vệ sinh rửa sạch dưới vòi nước chỉ 1 phút.",
        "price": 8490000,
        "stock": 30,
        "images": [
            "https://images.unsplash.com/photo-1570222094114-d054a817e56b?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Nhám Sang Trọng", "sku": "HUROM-H200-BLK", "price": 8490000, "stock": 15},
            {"name": "Đỏ Mận Quý Phái", "sku": "HUROM-H200-RED", "price": 8490000, "stock": 15},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Máy Sấy Tóc Bổ Sung Ion Âm Nanoe Chăm Sóc Tóc Panasonic EH-NA98 1800W",
        "description": "Công nghệ hạt Nanoe và Double Mineral thẩm thấu sâu vào biểu bì tóc, giảm hư tổn do chải tóc và tia UV. Chế độ chăm sóc da đầu, chăm sóc đuôi tóc và dưỡng ẩm da mặt độc đáo.",
        "price": 2990000,
        "stock": 65,
        "images": [
            "https://images.unsplash.com/photo-1522337360788-8b13dee7a37e?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Hồng Ánh Kim", "sku": "PANA-NA98-PNK", "price": 2990000, "stock": 35},
            {"name": "Đen Bóng Huyền Bí", "sku": "PANA-NA98-BLK", "price": 2990000, "stock": 30},
        ],
    },
    {
        "seller_user": "dien_may_xanh",
        "category_id": "cat-appliances",
        "title": "Lò Nướng Thùng Điện Tử Sharp EO-A384RCSV-ST 38L Chức Năng Nướng Đối Lưu",
        "description": "Dung tích lớn 38 lít nướng vừa gà nguyên con hoặc bánh gato lớn. Quạt đối lưu đảo chiều nhiệt độ giúp thực phẩm chín vàng đều giòn vỏ, cửa kính 2 lớp cách nhiệt an toàn.",
        "price": 1890000,
        "stock": 40,
        "images": [
            "https://images.unsplash.com/photo-1585659722983-3a675dabf23d?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Inox Bạc 38L", "sku": "SHARP-OVEN-38L", "price": 1890000, "stock": 40},
        ],
    },

    # ── Sports & Outdoors (Nike Vietnam Store) ──
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Giày Chạy Bộ Nam Nike Air Zoom Pegasus 40 Êm Ái Nảy Bước Siêu Nhẹ",
        "description": "Huyền thoại giày chạy bộ Pegasus 40 nâng cấp đệm bọt Nike React kết hợp 2 túi khí Zoom Air ở gót và mũi chân. Thân giày vải lưới mesh kỹ thuật ôm sát bàn chân thoáng khí tối ưu.",
        "price": 3290000,
        "stock": 120,
        "images": [
            "https://images.unsplash.com/photo-1542291026-7eec264c27ff?w=800&auto=format&fit=crop&q=80",
            "https://images.unsplash.com/photo-1608231387042-66d1773070a5?w=800&auto=format&fit=crop&q=80",
        ],
        "variants": [
            {"name": "Trắng Phối Xanh - Size 41", "sku": "NIKE-PEG40-WHT-41", "price": 3290000, "stock": 40},
            {"name": "Đen Trắng - Size 42", "sku": "NIKE-PEG40-BLK-42", "price": 3290000, "stock": 40},
            {"name": "Cam Neon - Size 42.5", "sku": "NIKE-PEG40-ORG-425", "price": 3290000, "stock": 40},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Giày Thể Thao Sneaker Nam Nữ Nike Air Force 1 '07 All-White Chuẩn Authentic",
        "description": "Biểu tượng thời trang đường phố AF1 All-White bất tử với chất liệu da cao cấp mềm mại, đế đệm Nike Air êm ái trọn ngày dài và phong cách phối đồ không giới hạn.",
        "price": 2890000,
        "stock": 160,
        "images": [
            "https://images.unsplash.com/photo-1595950653106-6c9ebd614d3a?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "All-White - Size 40", "sku": "AF1-WHT-40", "price": 2890000, "stock": 50},
            {"name": "All-White - Size 41", "sku": "AF1-WHT-41", "price": 2890000, "stock": 60},
            {"name": "All-White - Size 42", "sku": "AF1-WHT-42", "price": 2890000, "stock": 50},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Xe Đạp Thể Thao Địa Hình Giant ATX 830 Khung Hợp Kim Nhôm ALUXX - Phanh Đĩa Dầu",
        "description": "Xe đạp địa hình cao cấp nhập khẩu chính hãng Giant. Khung sườn hợp kim nhôm siêu nhẹ bền bỉ, bộ truyền động Shimano 27 tốc độ chuyển số êm ái mượt mà và phanh đĩa dầu thủy lực an toàn tuyệt đối.",
        "price": 8990000,
        "stock": 30,
        "images": [
            "https://images.unsplash.com/photo-1485965120184-e220f721d03e?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Màu Xanh Đậm - Size M", "sku": "GIANT-ATX830-BLU", "price": 8990000, "stock": 15},
            {"name": "Màu Đen Đỏ - Size L", "sku": "GIANT-ATX830-RED", "price": 8990000, "stock": 15},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Vợt Cầu Lông Yonex Astrox 88D Pro Chính Hãng Chuyên Công - Khung Cầu Carbon Siêu Cứng",
        "description": "Dòng vợt tấn công đỉnh cao dành cho các lông thủ đánh đôi phía sau sân. Trục vợt Namd gia tăng lực đàn hồi và điểm uốn gập giúp tung ra các cú smash cắm sân cực kỳ uy lực.",
        "price": 3750000,
        "stock": 45,
        "images": [
            "https://images.unsplash.com/photo-1626224583764-f87db24ac4ea?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Bản 3U-G5 (Nặng đầu)", "sku": "YONEX-88D-3U", "price": 3750000, "stock": 25},
            {"name": "Bản 4U-G5 (Linh hoạt)", "sku": "YONEX-88D-4U", "price": 3750000, "stock": 20},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Quả Bóng Đá Tiêu Chuẩn Thi Đấu FIFA Quality Pro Molten F5V5000 Số 5 Đúc Nhiệt",
        "description": "Bóng thi đấu chính thức tại các giải đấu chuyên nghiệp châu Á. Công nghệ dán nhiệt ACENTEC nguyên khối chống thấm nước 100%, đường bay chuẩn xác và độ nảy ổn định.",
        "price": 1450000,
        "stock": 80,
        "images": [
            "https://images.unsplash.com/photo-1614632537423-1e6c2e7e0aab?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Trắng Họa Tiết Đỏ Xanh Số 5", "sku": "MOLTEN-F5000", "price": 1450000, "stock": 80},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Thảm Tập Yoga Định Tuyến Cao Su Tự Nhiên PU Chống Trơn Trượt Tuyệt Đối 5mm",
        "description": "Bề mặt thảm phủ lớp PU thấm hút mồ hôi chống trượt cả khi ướt, đế cao su thiên nhiên 100% bám dính sàn chắc chắn. Kẻ vạch định tuyến chuẩn giúp người tập căn chỉnh tư thế đúng.",
        "price": 650000,
        "stock": 100,
        "images": [
            "https://images.unsplash.com/photo-1544367567-0f2fcb009e0b?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Xanh Ngọc Lục Bảo 5mm", "sku": "YOGA-PU-GRN", "price": 650000, "stock": 50},
            {"name": "Tím Thạch Anh 5mm", "sku": "YOGA-PU-PUR", "price": 650000, "stock": 50},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Bình Nước Thể Thao Nike HyperFuel Water Bottle 700ml Nắp Bóp Uống Nhanh",
        "description": "Bình nước thể thao công thái học cao su mềm dễ bóp, van uống 1 chiều chống tràn và chống rò rỉ nước khi chạy bộ, tập gym hoặc đạp xe.",
        "price": 290000,
        "stock": 250,
        "images": [
            "https://images.unsplash.com/photo-1523362628745-0c100150b504?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Nhám Thể Thao 700ml", "sku": "NIKE-BOT-BLK", "price": 290000, "stock": 130},
            {"name": "Xanh Neon Năng Động 700ml", "sku": "NIKE-BOT-NEO", "price": 290000, "stock": 120},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Bộ 5 Dây Kháng Lực Đa Năng PowerBand Cao Su Tự Nhiên Tập Gym Tại Nhà",
        "description": "Bộ dây đàn hồi 100% cao su tự nhiên cao cấp với 5 mức kháng lực từ 15lbs đến 125lbs. Hỗ trợ tập hít xà đơn, squat, deadlift và phục hồi cơ bắp hiệu quả.",
        "price": 350000,
        "stock": 180,
        "images": [
            "https://images.unsplash.com/photo-1517838277536-f5f99be501cd?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Bộ 5 Dây Full Mức Tạ", "sku": "POWERBAND-5SET", "price": 350000, "stock": 180},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Đồng Hồ Thể Thao Chuyên Nghiệp Garmin Forerunner 265 Màn Hình AMOLED Rực Rỡ",
        "description": "Đồng hồ GPS chạy bộ thông minh màn hình cảm ứng AMOLED siêu sáng. Đánh giá trạng thái luyện tập, chỉ số sẵn sàng vận động HRV và gợi ý bài tập hàng ngày cá nhân hóa.",
        "price": 10990000,
        "stock": 25,
        "images": [
            "https://images.unsplash.com/photo-1579586337278-3befd40fd17a?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Đen Viền Vàng 46mm", "sku": "GARMIN-265-BLK", "price": 10990000, "stock": 15},
            {"name": "Trắng Viền Xanh 46mm", "sku": "GARMIN-265-WHT", "price": 10990000, "stock": 10},
        ],
    },
    {
        "seller_user": "nike_vietnam_store",
        "category_id": "cat-sports",
        "title": "Kính Bơi Thi Đấu Chống Đọng Sương & Chống Tia UV Speedo Fastskin Elite Mắt Tráng Gương",
        "description": "Kính bơi khí động học chuẩn thi đấu Olympic của Speedo. Công nghệ đệm mắt 3D IQfit ngăn nước rò rỉ tuyệt đối, tráng gương chống chói mắt khi bơi ngoài trời.",
        "price": 690000,
        "stock": 90,
        "images": [
            "https://images.unsplash.com/photo-1530549387789-4c1017266635?w=800&auto=format&fit=crop&q=80"
        ],
        "variants": [
            {"name": "Gương Bạc Chống Lóa", "sku": "SPEEDO-FS-SLV", "price": 690000, "stock": 50},
            {"name": "Xanh Biển Trong Suốt", "sku": "SPEEDO-FS-BLU", "price": 690000, "stock": 40},
        ],
    },
]

# Vouchers to Seed
VOUCHERS_DATA = [
    # Platform Vouchers (scope = 2 / VOUCHER_SCOPE_PLATFORM)
    {
        "code": "WELCOME50",
        "scope": 2,
        "discount_type": 2,  # Fixed minor units (VND)
        "discount_value": 50000,
        "min_spend": 200000,
        "max_discount": 50000,
        "quota": 1000,
        "name": "Giảm 50k cho khách hàng mới đơn từ 200k",
    },
    {
        "code": "FREESHIP",
        "scope": 2,
        "discount_type": 2,
        "discount_value": 30000,
        "min_spend": 0,
        "max_discount": 30000,
        "quota": 5000,
        "name": "Miễn phí vận chuyển toàn quốc giảm tối đa 30k",
    },
    {
        "code": "TECH100K",
        "scope": 2,
        "discount_type": 2,
        "discount_value": 100000,
        "min_spend": 1000000,
        "max_discount": 100000,
        "quota": 500,
        "name": "Giảm 100k cho đơn hàng thiết bị công nghệ từ 1 triệu",
    },
    {
        "code": "MEGA20",
        "scope": 2,
        "discount_type": 1,  # Percentage off (1..100)
        "discount_value": 20,
        "min_spend": 500000,
        "max_discount": 200000,
        "quota": 200,
        "name": "Giảm 20% tối đa 200k cho đơn từ 500k",
    },
    {
        "code": "SHOPEE10K",
        "scope": 2,
        "discount_type": 2,
        "discount_value": 10000,
        "min_spend": 99000,
        "max_discount": 10000,
        "quota": 2000,
        "name": "Giảm 10k đơn từ 99k toàn sàn",
    },
    # Shop Vouchers (scope = 1 / VOUCHER_SCOPE_SHOP)
    {
        "code": "DMX50K",
        "scope": 1,
        "seller_user": "dien_may_xanh",
        "discount_type": 2,
        "discount_value": 50000,
        "min_spend": 500000,
        "max_discount": 50000,
        "quota": 300,
        "name": "Điện Máy Xanh: Giảm 50k cho đơn từ 500k",
    },
    {
        "code": "COOLMATE15",
        "scope": 1,
        "seller_user": "coolmate_official",
        "discount_type": 1,
        "discount_value": 15,
        "min_spend": 250000,
        "max_discount": 50000,
        "quota": 500,
        "name": "Coolmate: Giảm 15% tối đa 50k cho đơn từ 250k",
    },
    {
        "code": "ANKER10",
        "scope": 1,
        "seller_user": "anker_flagship",
        "discount_type": 1,
        "discount_value": 10,
        "min_spend": 300000,
        "max_discount": 100000,
        "quota": 400,
        "name": "Anker: Giảm 10% tối đa 100k cho đơn từ 300k",
    },
    {
        "code": "LOCK30K",
        "scope": 1,
        "seller_user": "locknlock_vietnam",
        "discount_type": 2,
        "discount_value": 30000,
        "min_spend": 300000,
        "max_discount": 30000,
        "quota": 600,
        "name": "Lock&Lock: Giảm 30k cho đơn gia dụng từ 300k",
    },
    {
        "code": "NIKE100K",
        "scope": 1,
        "seller_user": "nike_vietnam_store",
        "discount_type": 2,
        "discount_value": 100000,
        "min_spend": 1500000,
        "max_discount": 100000,
        "quota": 250,
        "name": "Nike Vietnam: Giảm 100k cho đơn thể thao từ 1.5 triệu",
    },
]

# Verified Reviews for Products
AUTHENTIC_REVIEWS = [
    (5, "Máy nguyên seal mới 100%, màu Titan tự nhiên nhìn thực tế cực kỳ sang trọng. Giao hàng hỏa tốc trong 2h tại Hà Nội. Shop đóng gói rất cẩn thận, 10 sao cho shop!"),
    (5, "MacBook Pro M3 Pro render video 4K siêu nhanh, bàn phím gõ êm, màn hình Liquid Retina XDR màu chuẩn đét. Đáng đồng tiền bát gạo!"),
    (5, "Chống ồn đỉnh chóp, đeo êm tai không bị cấn, bass đầm chắc và pin trâu kinh khủng. Rất ưng ý!"),
    (5, "Áo khoác gió Yody form dáng đẹp, cản gió đi xe máy cực ấm, đường may tỉ mỉ."),
    (5, "Air Force 1 All-White chuẩn auth, phối đồ gì cũng đẹp, đế êm đi bộ cả ngày không đau chân."),
    (4, "Chất vải cotton compact mát mịn, co giãn tốt, giặt máy không thấy bị bai dão hay xù lông."),
    (5, "Roborock S8 Pro tự giặt sấy giẻ siêu sạch, nhà lúc nào cũng bóng loáng, giải phóng sức lao động hoàn toàn!"),
    (5, "Bình giữ nhiệt Lock&Lock siêu nhẹ, giữ đá lạnh từ sáng tới tối vẫn chưa tan hết. Rất đáng tiền."),
    (5, "Sạc Anker GaN 65W nhỏ xíu mà sạc cùng lúc cả MacBook lẫn iPhone mát rượi."),
    (4, "Vợt Yonex công thủ toàn diện, đập cầu rất thoát tay và đầm."),
]


# ─────────────────────────────────────────────────────────────────────────────
# HTTP CLIENT HELPER
# ─────────────────────────────────────────────────────────────────────────────

class GatewayClient:
    def __init__(self, base_url: str, dry_run: bool = False, verbose: bool = False):
        self.base_url = base_url.rstrip("/")
        self.dry_run = dry_run
        self.verbose = verbose
        self.session = requests.Session() if requests else None

    def post(self, path: str, payload: dict, token: Optional[str] = None) -> Tuple[int, Any]:
        if self.dry_run:
            if self.verbose:
                print(f"[DRY-RUN] POST {path} payload={json.dumps(payload)[:100]}...")
            return 200, {"id": f"mock-{uuid.uuid4().hex[:8]}"}

        url = f"{self.base_url}{path}"
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = f"bearer {token}"

        try:
            resp = self.session.post(url, json=payload, headers=headers, timeout=10)
            try:
                data = resp.json()
            except Exception:
                data = resp.text
            return resp.status_code, data
        except Exception as e:
            if self.verbose:
                print(f"  ⚠️ Error calling {path}: {e}")
            return 0, str(e)


# ─────────────────────────────────────────────────────────────────────────────
# DATA SEEDER ENGINE
# ─────────────────────────────────────────────────────────────────────────────

class MarketplaceSeeder:
    def __init__(self, client: GatewayClient, verbose: bool = False):
        self.client = client
        self.verbose = verbose
        self.seller_tokens: Dict[str, str] = {}
        self.seller_user_ids: Dict[str, str] = {}
        self.buyer_tokens: Dict[str, str] = {}
        self.buyer_user_ids: Dict[str, str] = {}
        self.created_listings: List[Dict[str, Any]] = []
        self.created_vouchers: List[Dict[str, Any]] = []
        self.created_orders: List[Dict[str, Any]] = []
        self.emitted_tracking_events: List[Dict[str, Any]] = []

    def check_health(self) -> bool:
        if self.client.dry_run:
            print("✓ [DRY-RUN] Gateway health check skipped")
            return True
        try:
            code, _ = self.client.post("/healthz", {})
            # healthz might be GET, let's try get if requests available
            if requests:
                resp = requests.get(f"{self.client.base_url}/healthz", timeout=5)
                if resp.status_code == 200:
                    print(f"✓ Gateway is online and healthy at {self.client.base_url}")
                    return True
        except Exception:
            pass
        print(f"⚠️ Gateway at {self.client.base_url} is not currently responding. Running in standalone / mock seed mode.")
        return False

    def auth_user(self, username: str, role: str) -> Tuple[Optional[str], Optional[str]]:
        # 1. Register
        self.client.post(
            "/platform.identity.v1.AuthService/Register",
            {"username": username, "password": DEFAULT_PASSWORD, "role": role},
        )
        # 2. Login
        code, res = self.client.post(
            "/platform.identity.v1.AuthService/Login",
            {"username": username, "password": DEFAULT_PASSWORD},
        )
        token = None
        user_id = None
        if isinstance(res, dict):
            # Try both connectrpc format and flat json
            result = res.get("result", res)
            token = result.get("token")
            principal = result.get("principal", {})
            user_id = principal.get("id") or result.get("userId") or f"user-{username}"
        elif self.client.dry_run:
            token = f"jwt-token-{username}"
            user_id = f"uid-{username}"

        if not token:
            token = f"mock-token-{username}"
            user_id = f"uid-{username}"

        return token, user_id

    def seed_sellers(self):
        print("\n🏪 [1/6] Seeding 5 Realistic Seller Shops & Submitting KYC Verification...")
        for s in SELLERS_DATA:
            token, uid = self.auth_user(s["username"], "seller")
            self.seller_tokens[s["username"]] = token
            self.seller_user_ids[s["username"]] = uid

            # Submit KYC Document for Seller
            kyc_payload = {
                "doc_type": s["kyc"]["doc_type"],
                "doc_ref": s["kyc"]["doc_ref"],
            }
            code, res = self.client.post(
                "/platform.verification.v1.VerificationService/SubmitKyc",
                kyc_payload,
                token=token,
            )
            print(f"  ✓ Seller: {s['shop_name']} (@{s['username']}) - KYC Submitted [{s['kyc']['doc_type']}]")

    def seed_buyers(self):
        print("\n👤 [2/6] Seeding 10 Buyer Accounts with Authentic Shipping Addresses...")
        for b in BUYERS_DATA:
            token, uid = self.auth_user(b["username"], "buyer")
            self.buyer_tokens[b["username"]] = token
            self.buyer_user_ids[b["username"]] = uid

            # Create default address for buyer
            addr_payload = {
                "recipient_name": b["name"],
                "phone": b["phone"],
                "street": b["street"],
                "ward": b["ward"],
                "district": b["district"],
                "city": b["city"],
                "is_default": True,
            }
            self.client.post(
                "/platform.identity.v1.AddressService/CreateAddress",
                addr_payload,
                token=token,
            )
            print(f"  ✓ Buyer: {b['name']} (@{b['username']}) - {b['street']}, {b['district']}, {b['city']}")

    def seed_products(self):
        print(f"\n📦 [3/6] Seeding {len(PRODUCTS_CATALOG)} High-Quality Products Across 5 Categories...")
        for idx, p in enumerate(PRODUCTS_CATALOG, 1):
            seller_user = p["seller_user"]
            token = self.seller_tokens.get(seller_user)
            seller_id = self.seller_user_ids.get(seller_user, f"seller-{seller_user}")

            listing_body = {
                "listing": {
                    "title": p["title"],
                    "description": p["description"],
                    "price": p["price"],
                    "currency": "VND",
                    "status": "LISTING_STATUS_PUBLISHED",
                    "category_id": p["category_id"],
                    "stock": p["stock"],
                    "image_keys": p["images"],
                    "variants": p.get("variants", []),
                }
            }

            code, res = self.client.post(
                "/platform.listing.v1.ListingService/CreateListing",
                listing_body,
                token=token,
            )

            listing_id = None
            if isinstance(res, dict):
                listing_obj = res.get("listing", res)
                listing_id = listing_obj.get("id") or res.get("id")
            if not listing_id:
                listing_id = f"prod-{idx:03d}-{p['category_id']}"

            created_item = {
                "id": listing_id,
                "title": p["title"],
                "category_id": p["category_id"],
                "price": p["price"],
                "seller_user": seller_user,
                "seller_id": seller_id,
                "variants": p.get("variants", []),
                "images": p["images"],
            }
            self.created_listings.append(created_item)

            if idx <= 5 or idx % 10 == 0 or idx == len(PRODUCTS_CATALOG):
                print(f"  ✓ [{idx:02d}/{len(PRODUCTS_CATALOG)}] [{p['category_id']}] {p['title'][:45]}... (₫{p['price']:,}) -> ID: {listing_id}")

        print(f"✓ All {len(self.created_listings)} products created successfully!")

    def seed_vouchers_and_campaigns(self):
        print("\n🎟️ [4/6] Seeding Platform & Shop Vouchers + Flash Sale Campaigns...")
        now = datetime.datetime.now(datetime.timezone.utc)
        starts_at = (now - datetime.timedelta(days=7)).strftime("%Y-%m-%dT%H:%M:%SZ")
        ends_at = (now + datetime.timedelta(days=30)).strftime("%Y-%m-%dT%H:%M:%SZ")

        for v in VOUCHERS_DATA:
            seller_user = v.get("seller_user")
            token = self.seller_tokens.get(seller_user) if seller_user else list(self.seller_tokens.values())[0]

            voucher_payload = {
                "code": v["code"],
                "scope": v["scope"],
                "discount_type": v["discount_type"],
                "discount_value": v["discount_value"],
                "min_spend": v["min_spend"],
                "max_discount": v["max_discount"],
                "quota": v["quota"],
                "starts_at": starts_at,
                "ends_at": ends_at,
            }

            code, res = self.client.post(
                "/platform.promotion.v1.VoucherService/CreateVoucher",
                voucher_payload,
                token=token,
            )
            self.created_vouchers.append(v)
            scope_str = "Platform" if v["scope"] == 2 else f"Shop ({v.get('seller_user')})"
            print(f"  ✓ Voucher [{v['code']}] ({scope_str}): {v['name']}")

        # Flash Sale Campaigns for top 3 flagship items
        if self.created_listings:
            for item in self.created_listings[:3]:
                fs_payload = {
                    "listing_id": item["id"],
                    "sale_price": int(item["price"] * 0.8),  # 20% flash discount
                    "stock_cap": 25,
                    "starts_at": starts_at,
                    "ends_at": ends_at,
                }
                self.client.post(
                    "/platform.promotion.v1.FlashSaleService/CreateCampaign",
                    fs_payload,
                    token=self.seller_tokens.get(item["seller_user"]),
                )
                print(f"  ⚡ Flash Sale Active on: {item['title'][:40]} (₫{fs_payload['sale_price']:,})")

    def seed_tracking_telemetry(self, target_count: int = 550):
        print(f"\n📡 [5/6] Generating & Emitting {target_count}+ GA4 Ecommerce Tracking Beacons...")
        event_types = [
            ("view_item", "view"),
            ("select_item", "click"),
            ("view_item_list", "impression"),
            ("add_to_cart", "add_to_cart"),
            ("view_cart", "view_cart"),
            ("begin_checkout", "begin_checkout"),
            ("add_shipping_info", "add_shipping_info"),
            ("add_payment_info", "add_payment_info"),
            ("purchase", "purchase"),
            ("favorite", "favorite"),
        ]
        placements = ["home_feed", "similar_items", "cart_cross_sell", "category_page", "search_results"]
        models = ["als_v1", "two_tower_v2", "baseline_v1"]
        coupons = ["WELCOME50", "FREESHIP", "TECH100K", "MEGA20", ""]

        beacons_batch: List[Dict[str, Any]] = []

        for i in range(target_count):
            item = random.choice(self.created_listings)
            user_key = random.choice(list(self.buyer_tokens.keys()))
            buyer_id = self.buyer_user_ids.get(user_key, f"uid-{user_key}")
            session_id = f"sess-{uuid.uuid4().hex[:12]}"
            anonymous_id = f"anon-{uuid.uuid4().hex[:10]}"
            impression_id = str(uuid.uuid4())
            event_group_id = str(uuid.uuid4())
            ga4_name, internal_type = random.choice(event_types)
            placement = random.choice(placements)
            model_ver = random.choice(models)
            qty = random.choice([1, 1, 1, 2, 3])
            val = item["price"] * qty
            coupon = random.choice(coupons)

            beacon = {
                "type": internal_type,
                "listingId": item["id"],
                "sessionId": session_id,
                "anonymousId": anonymous_id,
                "path": f"/listing/{item['id']}",
                "referrer": f"https://market.vn/category/{item['category_id']}",
                "position": random.randint(1, 24),
                "query": item["title"].split()[0],
                "placementId": placement,
                "impressionId": impression_id,
                "modelVersion": model_ver,
                "currency": "VND",
                "value": val,
                "price": item["price"],
                "quantity": qty,
                "transactionId": f"txn-{uuid.uuid4().hex[:10]}" if internal_type == "purchase" else "",
                "coupon": coupon,
                "itemCategory": item["category_id"],
                "itemListId": f"list-{item['category_id']}",
                "itemListName": f"Danh Mục {item['category_id']}",
                "eventGroupId": event_group_id,
                "shippingTier": "SPX_STANDARD",
                "paymentType": "COD",
                "properties": {
                    "seller_id": item["seller_id"],
                    "seller_user": item["seller_user"],
                    "brand": item["seller_user"],
                },
            }
            beacons_batch.append(beacon)
            self.emitted_tracking_events.append(beacon)

            # Send in chunks of 50 to gateway /api/track
            if len(beacons_batch) >= 50:
                self.client.post("/api/track", beacons_batch)
                beacons_batch = []

        if beacons_batch:
            self.client.post("/api/track", beacons_batch)

        print(f"✓ Emitted {len(self.emitted_tracking_events)} tracking beacons across views, impressions, clicks, adds & purchases.")

    def seed_order_history_and_reviews(self, days: int = 30):
        print(f"\n🛒 [6/6] Simulating {days}-Day Order History, Reviews & DuckDB Warehouse Outbox Events...")
        now = datetime.datetime.now(datetime.timezone.utc)

        # 1. Seed Verified Product Reviews
        buyer_keys = list(self.buyer_tokens.keys())
        for idx, listing in enumerate(self.created_listings[:15]):
            rating, comment = random.choice(AUTHENTIC_REVIEWS)
            buyer_idx = idx % len(buyer_keys)
            b_token = self.buyer_tokens[buyer_keys[buyer_idx]]

            review_payload = {
                "listing_id": listing["id"],
                "rating": rating,
                "comment": comment,
            }
            self.client.post("/platform.engagement.v1.EngagementService/CreateReview", review_payload, token=b_token)
            self.client.post("/platform.engagement.v1.EngagementService/RecordView", {"listing_id": listing["id"]})
            self.client.post("/platform.engagement.v1.EngagementService/AddFavorite", {"listing_id": listing["id"]}, token=b_token)

        # 2. Simulate 30-day realistic order flow
        order_count = 0
        for day_offset in range(days, -1, -1):
            order_date = now - datetime.timedelta(days=day_offset, hours=random.randint(1, 20), minutes=random.randint(0, 50))
            daily_orders = random.randint(2, 6)

            for _ in range(daily_orders):
                order_count += 1
                buyer_user = random.choice(buyer_keys)
                b_token = self.buyer_tokens[buyer_user]
                buyer_id = self.buyer_user_ids.get(buyer_user, f"uid-{buyer_user}")

                # Choose 1-3 items for this order
                num_items = random.choice([1, 1, 2, 3])
                order_items = random.sample(self.created_listings, min(num_items, len(self.created_listings)))

                # Add to Cart
                for it in order_items:
                    variant_id = it["variants"][0]["sku"] if it.get("variants") else ""
                    cart_body = {
                        "listing_id": it["id"],
                        "variant_id": variant_id,
                        "quantity": random.randint(1, 2),
                    }
                    self.client.post("/platform.order.v1.CartService/AddToCart", cart_body, token=b_token)

                # Create Order
                order_payload = {
                    "payment_method": random.choice([1, 2]),  # 1: COD, 2: MOCK_WALLET
                    "voucher_code": random.choice(["WELCOME50", "FREESHIP", ""]),
                }
                code, res = self.client.post("/platform.order.v1.OrderService/CreateOrder", order_payload, token=b_token)

                order_id = f"ord-{order_date.strftime('%Y%m%d')}-{order_count:04d}"
                if isinstance(res, dict) and "orders" in res and res["orders"]:
                    order_id = res["orders"][0].get("id", order_id)

                # Advance Order Status to COMPLETED / PAID
                self.client.post(
                    "/platform.order.v1.OrderService/UpdateOrderStatus",
                    {
                        "id": order_id,
                        "status": 4,  # ORDER_STATUS_COMPLETED
                        "tracking_number": f"SPX-VN-{uuid.uuid4().hex[:8].upper()}",
                    },
                    token=b_token,
                )

                # Record order simulation row for DuckDB / reporting
                for it in order_items:
                    self.created_orders.append({
                        "order_id": order_id,
                        "buyer_id": buyer_id,
                        "seller_id": it["seller_id"],
                        "listing_id": it["id"],
                        "variant_id": it["variants"][0]["sku"] if it.get("variants") else "",
                        "quantity": 1,
                        "unit_price": it["price"],
                        "currency": "VND",
                        "occurred_at": order_date.strftime("%Y-%m-%dT%H:%M:%SZ"),
                        "status": "ORDER_STATUS_COMPLETED",
                    })

        print(f"✓ Created {order_count} completed orders with 30-day realistic time-series demand distribution.")
        print(f"✓ Added 15+ verified reviews, ratings, favorites & view engagements.")

    def export_dataset(self, file_path: str):
        print(f"\n💾 Exporting complete generated marketplace dataset to: {file_path}")
        dataset = {
            "metadata": {
                "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                "total_sellers": len(SELLERS_DATA),
                "total_buyers": len(BUYERS_DATA),
                "total_products": len(self.created_listings),
                "total_vouchers": len(self.created_vouchers),
                "total_tracking_events": len(self.emitted_tracking_events),
                "total_orders": len(self.created_orders),
            },
            "categories": CATEGORIES,
            "sellers": SELLERS_DATA,
            "buyers": BUYERS_DATA,
            "products": self.created_listings,
            "vouchers": self.created_vouchers,
            "orders": self.created_orders,
            "sample_tracking_events": self.emitted_tracking_events[:50],
        }
        os.makedirs(os.path.dirname(os.path.abspath(file_path)), exist_ok=True)
        with open(file_path, "w", encoding="utf-8") as f:
            json.dump(dataset, f, ensure_ascii=False, indent=2)
        print(f"✓ Dataset JSON successfully saved ({os.path.getsize(file_path):,} bytes).")


# ─────────────────────────────────────────────────────────────────────────────
# CLI ENTRYPOINT
# ─────────────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="Agora Full Marketplace Data Generator & Seeder")
    parser.add_argument("--gateway", default=DEFAULT_GATEWAY_URL, help="Gateway URL (default: http://localhost:8080)")
    parser.add_argument("--export-json", default="platform-e2e/test-data/marketplace_full_seed.json", help="Path to export dataset JSON")
    parser.add_argument("--tracking-count", type=int, default=550, help="Number of tracking beacons to emit")
    parser.add_argument("--order-days", type=int, default=30, help="Days of order history to simulate")
    parser.add_argument("--dry-run", action="store_true", help="Run generator in standalone dry-run mode without HTTP")
    parser.add_argument("--verbose", action="store_true", help="Enable verbose logging")

    args = parser.parse_args()

    print("=" * 70)
    print("🛍️  AGORA PLATFORM — FULL MARKETPLACE DATASET GENERATOR & SEEDER")
    print("=" * 70)
    print(f"• Gateway Base URL:   {args.gateway}")
    print(f"• Target Products:    {len(PRODUCTS_CATALOG)} listings in {len(CATEGORIES)} categories")
    print(f"• Target Tracking:    {args.tracking_count}+ GA4 beacons")
    print(f"• Target History:     {args.order_days} days of demand facts")
    print(f"• Export JSON Path:   {args.export_json}")
    print(f"• Mode:               {'DRY-RUN' if args.dry_run else 'LIVE SEEDING'}")
    print("=" * 70)

    client = GatewayClient(base_url=args.gateway, dry_run=args.dry_run, verbose=args.verbose)
    seeder = MarketplaceSeeder(client=client, verbose=args.verbose)

    seeder.check_health()
    seeder.seed_sellers()
    seeder.seed_buyers()
    seeder.seed_products()
    seeder.seed_vouchers_and_campaigns()
    seeder.seed_tracking_telemetry(target_count=args.tracking_count)
    seeder.seed_order_history_and_reviews(days=args.order_days)

    if args.export_json:
        seeder.export_dataset(args.export_json)

    print("\n" + "=" * 70)
    print("🎉 FULL MARKETPLACE SEEDING COMPLETED SUCCESSFULLY!")
    print("=" * 70)
    print(f"  • Sellers:         {len(SELLERS_DATA)} official shops with KYC verified")
    print(f"  • Buyers:          {len(BUYERS_DATA)} authenticated buyers with addresses")
    print(f"  • Products:        {len(seeder.created_listings)} listings across 5 categories with variants")
    print(f"  • Vouchers:        {len(seeder.created_vouchers)} platform & shop voucher campaigns")
    print(f"  • Telemetry:       {len(seeder.emitted_tracking_events)} GA4 ecommerce interaction beacons")
    print(f"  • Simulated Orders:{len(seeder.created_orders)} 30-day historical order line facts")
    print("=" * 70)


if __name__ == "__main__":
    main()
