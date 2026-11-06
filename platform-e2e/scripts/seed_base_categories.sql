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
ON CONFLICT DO NOTHING;
