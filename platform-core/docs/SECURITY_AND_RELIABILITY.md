# BẢN THIẾT KẾ KIẾN TRÚC BẢO MẬT & ĐỘ TIN CẬY (SECURITY & SRE RELIABILITY)

Tài liệu thiết kế mô hình Zero-Trust Security, phòng chống tấn công, khả năng chịu lỗi (Resilience) và chiến lược phục hồi thảm họa (Disaster Recovery).

---

## 🔒 1. MÔ HÌNH BẢO MẬT ZERO-TRUST & XÁC THỰC PHÂN QUYỀN

```
[ BROWSER / CLIENT ]
        │
        ▼ (Bearer JWT in HTTP Authorization header / Session Cookie)
┌────────────────────────────────────────────────────────────────────────┐
│                        team-gateway (:8080)                            │
│ 1. Xác thực chữ ký HS256 / RS256 JWT một lần duy nhất tại Edge         │
│ 2. Giải mã Claims -> Principal { id, type, scopes }                   │
│ 3. Bỏ header Authorization của client, TẠO MỚI gRPC Metadata:          │
│    - x-principal-id: "usr_abc"                                         │
│    - x-principal-type: "seller"                                        │
│    - x-principal-scopes: "listing.read,listing.write"                  │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │
                                    ▼ (gRPC + mTLS với Trusted Metadata)
┌────────────────────────────────────────────────────────────────────────┐
│                        DOWNSTREAM MICROSERVICES                        │
│ 1. Interceptor đọc x-principal-* metadata                              │
│ 2. Kiểm tra quyền hạn bằng RequireScopes("listing.write")              │
│ 3. Nếu thiếu scope -> Trả về PermissionDenied (gRPC Code 7)            │
└────────────────────────────────────────────────────────────────────────┘
```

### Ưu Điểm Của Mô Hình:
- **Client Không Thể Giả Mạo Quyền (Spoofing-Proof)**: Gateway luôn ghi đè và dựng lại `x-principal-*` metadata cho mỗi hop gọi nội bộ, không bao giờ chuyển tiếp trực tiếp header từ client.
- **Tiết Kiệm Chi Phí CPU**: Các service backend không phải lặp lại việc verify chữ ký cryptographic JWT cho từng request con.

---

## 🛡️ 2. RATE LIMITING & CHỐNG TẤN CÔNG DDOS

1. **Thuật Toán Token Bucket**:
   - Áp dụng tại `team-gateway` qua thư viện `golang.org/x/time/rate`.
   - **Mặc định**: 100 requests/giây (Burst: 200) cho mỗi IP / Authenticated User.
   - **Xử lý khi vượt ngưỡng**: Trả về ngay `429 Too Many Requests` kèm header `Retry-After: 1` trước khi request chạm vào backend services.
2. **CORS & CSRF Protection**:
   - `CORS_ORIGINS` giới hạn nghiêm ngặt domain frontend tin cậy (`http://localhost:3000`, `https://market.vn`).
   - Cookie xác thực mang cờ `httpOnly; SameSite=Lax; Secure`.

---

## ⚡ 3. KHẢ NĂNG PHỤC HỒI & CHỊU LỖI (RESILIENCE & CIRCUIT BREAKERS)

```
[ Request ] ──► [ Gateway / Client ] ──► [ gRPC Timeout Deadline: 2s ]
                                                │
                                                ├──(Lỗi tạm thời: UNAVAILABLE)──► [ Retry với Jitter ]
                                                │
                                                └──(Timeout / Sập service)──► [ Fallback Degradation ]
```

1. **gRPC Context Deadlines**: Mọi request xuyên service đều có deadline tối đa (2000ms). Tránh tình huống 1 service bị nghẽn làm treo cả chuỗi gọi (Cascading Failure).
2. **Graceful Degradation (Giảm cấp dịch vụ an toàn)**:
   - Nếu `team-search` (OpenSearch) gặp sự cố quá tải $\rightarrow$ Gateway tự động fallback đọc trực tiếp từ `team-domain` theo ID hoặc trả về dữ liệu cache Redis gần nhất kèm thông báo nhẹ nhàng cho người dùng.
   - Nếu AI Recommendation service chậm $\rightarrow$ Tự động fallback về danh sách sản phẩm mới nhất thay vì báo lỗi toàn trang.

---

## 💾 4. CHIẾN LƯỢC SAO LƯU & PHỤC HỒI THẢM HỌA (DISASTER RECOVERY)

| Chỉ số / Thành phần | Mục tiêu cam kết (SLO/SLA) | Phương pháp thực thi |
|---|---|---|
| **RPO (Recovery Point Objective)** | **< 1 Phút** | Continuous WAL Archiving của PostgreSQL đẩy lên S3/GCS; Kafka Topic log replication factor = 3 |
| **RTO (Recovery Time Objective)** | **< 15 Phút** | Tự động hóa triển khai lại toàn bộ hạ tầng bằng Terraform & ArgoCD GitOps |
| **OpenSearch Disaster Recovery** | Tự phục hồi 100% | OpenSearch là Read-Model: Khi mất toàn bộ cluster, chỉ cần chạy Replay Consumer đọc lại Kafka từ offset 0 để build lại index từ đầu mà không mất 1 byte dữ liệu |
| **Database Backup Cadence** | Hàng ngày + Streaming | Snapshot Full DB lúc 02:00 sáng hàng ngày + WAL Streaming liên tục 5 phút/lần |
