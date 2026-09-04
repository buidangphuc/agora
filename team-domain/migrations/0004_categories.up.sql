-- 0004_categories — Category tree & product tagging (Phase 3).

CREATE TABLE IF NOT EXISTS categories (
    id            TEXT PRIMARY KEY,
    name          TEXT        NOT NULL,
    slug          TEXT        NOT NULL UNIQUE,
    parent_id     TEXT        REFERENCES categories(id) ON DELETE SET NULL,
    display_order INT         NOT NULL DEFAULT 0,
    icon_url      TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS categories_parent_idx ON categories (parent_id);
CREATE INDEX IF NOT EXISTS categories_order_idx ON categories (display_order);

-- Add category_id to listings table
ALTER TABLE listings ADD COLUMN IF NOT EXISTS category_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS listings_category_idx ON listings (category_id);

-- Seed standard top-level categories
INSERT INTO categories (id, name, slug, parent_id, display_order, icon_url)
VALUES
    ('cat-electronics', 'Điện tử & Công nghệ', 'dien-tu-cong-nghe', NULL, 1, '📱'),
    ('cat-fashion', 'Thời trang & Phụ kiện', 'thoi-trang-phu-kien', NULL, 2, '👕'),
    ('cat-home', 'Nhà cửa & Đời sống', 'nha-cua-doi-song', NULL, 3, '🏠'),
    ('cat-beauty', 'Sắc đẹp & Sức khỏe', 'sac-dep-suc-khoe', NULL, 4, '💄'),
    ('cat-sports', 'Thể thao & Du lịch', 'the-thao-du-lich', NULL, 5, '⚽'),
    ('cat-books', 'Sách & Văn phòng phẩm', 'sach-van-phong-pham', NULL, 6, '📚'),
    ('cat-food', 'Bách hóa & Thực phẩm', 'bach-hoa-thuc-pham', NULL, 7, '🍎'),
    ('cat-appliances', 'Thiết bị gia dụng', 'thiet-bi-gia-dung', NULL, 8, '🔌')
ON CONFLICT (id) DO NOTHING;
