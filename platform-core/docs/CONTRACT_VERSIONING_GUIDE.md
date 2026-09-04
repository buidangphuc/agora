# HƯỚNG DẪN QUẢN LÝ PHIÊN BẢN HỢP ĐỒNG (CONTRACT VERSIONING & EVOLUTION GUIDE)

Tài liệu hướng dẫn quy chuẩn quản lý vòng đời và tiến hóa hợp đồng gRPC / Protobuf (`v1` $\rightarrow$ `v2`) để đảm bảo hệ thống mở rộng không bao giờ bị đứt gãy tương thích ngược (Zero Downtime).

---

## 🎯 1. NGUYÊN TẮC BẤT BIẾN (IMMUTABILITY RULES)

1. **Không Bao Giờ Xóa Hoặc Đổi Số Thứ Tự Field (Tag Number)** trong cùng 1 package:
   ```protobuf
   // ❌ SAI (Làm gãy binary protobuf parsing của consumer cũ):
   message Listing {
     string title = 2; // Đổi số 1 -> 2
   }

   // ✅ ĐÚNG (Thêm trường mới với số thứ tự tiếp theo):
   message Listing {
     string id = 1;
     string title = 2;
     string brand = 12; // Thêm trường mới
   }
   ```
2. **`buf breaking` Kiểm Soát Tự Động**:
   - Mọi Pull Request sửa đổi `.proto` đều được chạy `buf breaking --against '.git#branch=main'`. Nếu có breaking change trong cùng package, CI sẽ lập tức chặn merge.

---

## 🚀 2. QUY TRÌNH TIẾN HÓA LÊN PHIÊN BẢN MỚI (`v1` $\rightarrow$ `v2`)

Khi có yêu cầu tái cấu trúc lớn không thể duy trì tương thích trong `v1`, ta áp dụng chiến lược **Song Hành Đa Phiên Bản (Dual-Stack Versioning)**:

```
platform-core/packages/proto/platform/listing/
├── v1/
│   └── listing.proto   # package platform.listing.v1; (Đang phục vụ clients cũ)
└── v2/
    └── listing.proto   # package platform.listing.v2; (Phiên bản kiến trúc mới)
```

### Bước 1: Khai Báo Package `v2` Song Song
Tạo thư mục `v2/listing.proto` với package `platform.listing.v2`:
```protobuf
syntax = "proto3";

package platform.listing.v2;

service ListingService {
  rpc GetProduct(GetProductRequest) returns (GetProductResponse);
  rpc CreateProduct(CreateProductRequest) returns (CreateProductResponse);
}
```

### Bước 2: Server Triển Khai Cả 2 Interface (Dual-Stack gRPC Registration)
Trong `team-domain/cmd/server/main.go`:
```go
// Đăng ký phục vụ cả v1 và v2 trên cùng 1 gRPC port
listingv1.RegisterListingServiceServer(grpcServer, v1Handler)
listingv2.RegisterListingServiceServer(grpcServer, v2Handler)
```

### Bước 3: Định Tuyến Tại Gateway & Chuyển Đổi Consumer Dần Dần (Canary / Phased Rollout)
- `team-gateway` tiếp nhận cả route `/platform.listing.v1.ListingService/*` và `/platform.listing.v2.ListingService/*`.
- Các mobile app cũ hoặc consumer cũ tiếp tục gọi `v1`.
- Web frontend hoặc mobile app mới chuyển sang gọi `v2`.

### Bước 4: Khai Tử (Deprecation & Sunsetting) `v1`
1. Đánh dấu `option deprecated = true;` trong `v1.proto`.
2. Giám sát metrics trên Prometheus: Khi traffic của `v1` về 0% $\rightarrow$ Gỡ bỏ `v1` an toàn.

---

## 🛠️ 3. LỆNH ĐỒNG BỘ TOÀN BỘ PROTO TOÀN SÀN

Bất kỳ khi nào bạn cập nhật proto, chỉ cần chạy đúng 1 lệnh:
```bash
./platform-core/tools/sync-proto.sh
```
Tool sẽ tự động:
1. `buf lint` kiểm tra chuẩn cú pháp.
2. Đồng bộ sang tất cả 5 repos consumer.
3. Sinh mã Go & TypeScript mới nhất.
