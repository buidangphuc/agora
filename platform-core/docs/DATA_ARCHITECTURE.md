# BẢN THIẾT KẾ KIẾN TRÚC DỮ LIỆU & SỰ KIỆN (DATA PLATFORM & EVENT-DRIVEN CQRS)

Tài liệu thiết kế luồng dữ liệu, tính toàn vẹn (Data Consistency), xử lý sự kiện bất đồng bộ và cơ chế bộ nhớ đệm (Caching) cho hệ thống sàn thương mại điện tử.

---

## 🌊 1. TỔNG QUAN KIẾN TRÚC DỮ LIỆU (CQRS + EVENT-DRIVEN)

```
[ SELLER / WRITER ]
       │
       ▼
 ┌───────────┐         (Atomic DB Tx)          ┌────────────────┐
 │team-domain│ ───────────────────────────────►│  PostgreSQL     │
 └─────┬─────┘                                 │  - listings    │
       │                                       │  - outbox      │
       │ (Transactional Outbox / CDC)          └───────┬────────┘
       ▼                                               │
 ┌───────────┐        (Kafka Event Streaming)         │
 │Debezium / │ ───────────────────────────────────────┤
 │Publisher  │                                         ▼
 └─────┬─────┘                       ┌───────────────────────────────────┐
       │                             │ Kafka Partitioned Topics          │
       ├────────────────────────────►│ - listing.base.events (Key=ID)    │
       ├────────────────────────────►│ - listing.pricing.events (Key=ID) │
       ├────────────────────────────►│ - listing.stock.events (Key=ID)   │
       └────────────────────────────►│ - listing.status.events (Key=ID)  │
                                     └─────────────────┬─────────────────┘
                                                       │
                                                       ▼ (Idempotent Consumer)
                                              ┌─────────────────┐
                                              │   team-search   │
                                              └────────┬────────┘
                                                       │ (Partial Update)
                                                       ▼
                                              ┌─────────────────┐
                                              │   OpenSearch    │
                                              │   (Read-Model)  │
                                              └─────────────────┘
```

### Topic sự kiện hành vi (Analytics stream)

Tách hoàn toàn khỏi các DB giao dịch (Postgres) và khỏi các topic `listing.*.events`:
sự kiện hành vi (view/click/add-to-cart/impression) có lưu lượng gấp 500–1000× đơn hàng
nên đi trên một stream riêng nạp vào data warehouse (DuckDB local / BigQuery prod).

| Topic Kafka        | Payload type                              |
| ------------------ | ----------------------------------------- |
| `analytics.events` | `platform.analytics.v1.TrackingEvent` (bọc trong `platform.events.v1.EventEnvelope`) |

---

## 🛡️ 2. TRANSACTIONAL OUTBOX PATTERN (CHỐNG MẤT MÁT DỮ LIỆU / NO DUAL-WRITE)

### Vấn Đề "Dual-Write":
Nếu service vừa ghi vào PostgreSQL vừa gọi Kafka:
1. Ghi DB thành công nhưng Kafka bị timeout $\rightarrow$ Mất sự kiện, OpenSearch không nhận được cập nhật.
2. Bắn Kafka thành công nhưng DB bị rollback $\rightarrow$ Dữ liệu ma (Phantom data) trên Search.

### Giải Pháp Transactional Outbox:
1. Trong cùng **1 Database Transaction** trên PostgreSQL:
   ```sql
   BEGIN;
   -- 1. Cập nhật bảng nghiệp vụ
   UPDATE listings SET price = 27990000 WHERE id = 'prod-123';

   -- 2. Ghi sự kiện vào bảng outbox_events
   INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, payload, created_at)
   VALUES (
     gen_random_uuid(),
     'Listing',
     'prod-123',
     'platform.listing.v1.ListingPricingChanged',
     '{"listing_id":"prod-123","original_price":29990000,"promotional_price":27990000,"is_on_sale":true}',
     NOW()
   );
   COMMIT;
   ```
2. **Debezium CDC (Change Data Capture)** hoặc Outbox Poller đọc WAL (Write-Ahead Log) của PostgreSQL và đẩy tin cậy vào Kafka với đảm bảo **At-Least-Once Delivery**.

---

## 🔑 3. CHIẾN LƯỢC PARTITIONING & KEYING CHO KAFKA

- **Keying Strategy**: Luôn gán **`Key = listing_id`** cho mọi record trên tất cả các topic `listing.*.events`.
- **Đảm bảo thứ tự (Per-Entity Ordering)**: Mọi sự kiện của cùng 1 sản phẩm (BaseInfo $\rightarrow$ Pricing $\rightarrow$ Stock) luôn rơi vào **cùng 1 Partition** của Kafka, đảm bảo thứ tự thời gian được bảo toàn tuyệt đối, loại bỏ hoàn toàn race conditions.
- **Partition Count**: 12 Partitions cho mỗi topic để hỗ trợ scale lên tới 12 Consumer Pods chạy song song.

---

## 🧯 4. DEAD LETTER QUEUE (DLQ) & RETRY POLICY

```
[ Kafka Topic ] ──► [ Consumer ] ──► [ Try Process (1) ]
                           │
                           ├──(Thất bại)──► [ Exponential Backoff Retry (Max 3 lần: 1s, 2s, 4s) ]
                           │
                           └──(Vẫn lỗi)──► [ Dead Letter Queue: listing.events.DLQ ]
                                                         │
                                                         ▼
                                            [ Alert SRE + Retry Dashboard ]
```

1. **Retry với Exponential Backoff & Jitter**: Tự động thử lại tối đa 3 lần với khoảng cách $1s \pm 20\%$, $2s \pm 20\%$, $4s \pm 20\%$.
2. **Poison Pill Handling**: Nếu sau 3 lần vẫn lỗi (do parse lỗi hoặc payload hỏng), message sẽ được chuyển vào topic Dead Letter Queue `listing.events.DLQ` kèm header chứa `x-error-reason`, `x-original-topic`, `x-retry-count`.
3. **Consumer không bị Block**: Luồng xử lý chính tiếp tục vận hành mà không bị dừng lại bởi 1 message lỗi.

---

## 🚀 5. CHIẾN LƯỢC BỘ NHỚ ĐỆM ĐA TẦNG (MULTI-TIER CACHING)

```
[ Request ] ──► [ L1: Local In-Memory Cache (Go BigCache / SyncMap - TTL 5s) ]
                      │ (Miss)
                      ▼
                [ L2: Distributed Redis Cluster (TTL 10m) ]
                      │ (Miss)
                      ▼
                [ L3: OpenSearch / PostgreSQL Source ]
```

- **L1 In-Memory Cache (Microsecond Latency)**: Lưu trữ các metadata tĩnh (Danh mục sản phẩm, Cấu hình hệ thống).
- **L2 Redis Distributed Cache (Millisecond Latency)**: Lưu trữ Top 100 sản phẩm hot, chi tiết Flash sale đếm ngược, Session người dùng.
- **Cơ chế Cache Invalidation Chủ Động (Event-Driven Invalidation)**:
  - Khi nhận `ListingPricingChanged` hoặc `ListingBaseInfoChanged` từ Kafka $\rightarrow$ Consumer xóa key `cache:listing:{id}` trên Redis ngay lập tức $\rightarrow$ Đảm bảo cache luôn tươi mới mà không phụ thuộc hoàn toàn vào TTL.
