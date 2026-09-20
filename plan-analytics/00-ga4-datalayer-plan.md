# Plan — GA4 / GTM Data Layer pattern cho Agora

> Trạng thái: **PLAN ONLY** (chưa code). Mọi wave dưới đây là 1 OpenSpec change riêng.
> Repo đích: `~/Documents/agora` (checkout active). `full_team_repo` là snapshot — đồng bộ
> sau bằng `push-agora-snapshot.sh`.

---

## 0. Quyết định kiến trúc (đã chốt)

| # | Quyết định | Chốt | Hệ quả |
|---|---|---|---|
| D1 | Repo đích | `~/Documents/agora` | full_team_repo chỉ nhận snapshot ở cuối |
| D2 | Canonical event name | **Giữ enum nội bộ** (`view/click/add_to_cart/...`), thêm **map layer** 2 chiều | Zero breaking: `recsys/weights.py`, `query/duckdb.go` funnel SQL, e2e assertions, dữ liệu warehouse cũ đều không đổi. GA4 name chỉ xuất hiện ở (a) `dataLayer` push trên browser, (b) view `ga4_events` trong warehouse |
| D3 | Mô hình `items[]` | **Fan-out phẳng: 1 row = 1 item**, client tách `items[]` thành N beacon chung `event_group_id` | Giữ 1 bảng phẳng; recsys + funnel SQL không cần JOIN. Revenue dedupe bằng `transaction_id` |
| D4 | Phạm vi | **Full stack W0 → W5** | 5 repo, mỗi wave additive + có gate riêng |

**Nguyên tắc bất di bất dịch**

1. `order_facts` (team-order, transition PAID) vẫn là **nguồn doanh thu authoritative duy nhất**.
   `purchase` từ FE chỉ phục vụ funnel/marketing. **Không bao giờ** SUM tiền từ `tracking_events`.
2. Payload **không chứa PII** (contract rule hiện hành). Identity đi trong `EventEnvelope.principal`.
   → **không gửi `item_name`** từ browser; join từ listing dimension khi cần.
3. Gateway **dumb** (AGENTS.md Rule 2): edge chỉ parse + map + stamp principal + produce.
   Logic tách `items[]` nằm ở client.
4. GTM **tắt mặc định** (`NEXT_PUBLIC_GTM_ID` rỗng) → dev/CI/e2e không gọi mạng ngoài.
5. Telemetry **không bao giờ** throw vào caller (giữ đúng contract của `track.ts` hiện tại).

---

## 1. Hiện trạng đã verify

| Tầng | Đang có | Vị trí |
|---|---|---|
| FE transport | `track()` — sendBeacon + keepalive fetch fallback, text/plain (tránh preflight), never-throw, no-op khi SSR | `team-frontend/src/lib/track.ts` |
| FE call sites | **chỉ 6 file** import `@/lib/track` | `features/tracking/{TrackView,TrackLink,TrackImpression,SearchImpressions}.tsx`, `features/cart/AddToCartButton.tsx`, `features/order/CheckoutView.tsx` |
| Edge collector | parse single **hoặc array**, validate toàn bộ trước khi produce (atomic reject), stamp principal từ Bearer **hoặc cookie `session`**, luôn trả 204 | `team-gateway/internal/edge/collector.go` (`beaconEventTypes` :39, `HandleTrack` :58, `parseBeacons` :120, `beaconPrincipal` :152) |
| Contract | `EventType` 0..10, `TrackingEvent` field 1..12 (đã có `placement_id`/`impression_id`/`model_version`/`position`) | `platform-core/packages/proto/platform/analytics/v1/analytics.proto` |
| Warehouse | `TrackingRecord` + `Schema` 16 cột, parity test DuckDB↔BigQuery | `team-analytics/internal/warehouse/warehouse.go` (:17, :63) |
| Consumer | `RecordFromEnvelope` switch theo envelope type, `eventTypeName` chuẩn hoá lowercase | `team-analytics/internal/consumer/tracking.go` (:23, :59) |
| Query | funnel SQL `COUNT(*) FILTER (WHERE event_type=...)` | `team-analytics/internal/query/duckdb.go:30` |
| Downstream | `read_tracking_events` → implicit feedback triples; `event_weight` map theo tên event | `platform-recsys/recsys/{warehouse,interactions,weights}.py` |
| E2E | `consume_tracking_events` từ Kafka | `platform-e2e/tests/e2e/flows/tracking_flow.py`, `step_definitions/tracking_steps.py` |
| GTM / gtag / dataLayer | **KHÔNG CÓ** (grep sạch trong `src/`) | — |

---

## 2. Gap phân tích

| # | Gap | Bằng chứng | Mức |
|---|---|---|---|
| G1 | **Không có event bus.** Component gọi thẳng `track()` → coupling 1–1 với 1 destination. Thêm GA4/Meta = sửa từng component | 6 call site import trực tiếp `@/lib/track` | **Cao** — đây chính là tầng 1 của pattern |
| G2 | **Không có tiền trong schema.** Thiếu `currency`, `value`, `price`, `quantity`, `transaction_id`, `coupon`, `item_category`, `item_list_id` | `analytics.proto` field 1..12 | **Cao** |
| G3 | **Không batching client-side.** `SearchImpressions` bắn **N request cho N kết quả** dù collector đã nhận array | `SearchImpressions.tsx:20` `listingIds.forEach(... track(...))` | **Cao** — bug hiệu năng thật |
| G4 | **Funnel cụt.** `GetSellerFunnel` chỉ impressions/views/adds | `analytics.proto` `GetSellerFunnelResponse`, `duckdb.go:30` | Trung bình |
| G5 | **Thiếu 4 bước funnel GA4:** `view_cart`, `add_shipping_info`, `add_payment_info`, `purchase` | `EventType` enum hiện 10 giá trị | Trung bình |
| G6 | **Warehouse không migrate được.** Adapter chỉ `CREATE TABLE IF NOT EXISTS` → file `.duckdb`/bảng BQ đang tồn tại **không tự có cột mới** | `warehouse/duckdb/duckdb.go:50` | **Cao** — rủi ro ẩn, dễ bỏ sót |
| G7 | Không có consent gate / Consent Mode | — | Thấp (defer W5) |

---

## 3. Wave plan

### W0 · Contract — `platform-core`
**Mục tiêu:** contract đủ chỗ chứa ecommerce, additive 100%.

- `packages/proto/platform/analytics/v1/analytics.proto`:
  - Thêm `EventType`: `EVENT_TYPE_VIEW_CART = 11`, `EVENT_TYPE_ADD_SHIPPING_INFO = 12`,
    `EVENT_TYPE_ADD_PAYMENT_INFO = 13`, `EVENT_TYPE_PURCHASE = 14`.
    → **Không** thêm `VIEW_ITEM_LIST`/`SELECT_ITEM`: `IMPRESSION`/`CLICK` đã cover, chỉ khác tên (D2).
  - Thêm field vào `TrackingEvent` (13→):
    | # | Field | Ghi chú |
    |---|---|---|
    | 13 | `string currency` | ISO-4217, mặc định `VND` |
    | 14 | `int64 value` | **minor units**, event-level (GA4 `value`) |
    | 15 | `int64 price` | minor units, item-level |
    | 16 | `uint32 quantity` | item-level |
    | 17 | `string transaction_id` | khoá dedupe cho `purchase` |
    | 18 | `string coupon` | mã voucher áp dụng |
    | 19 | `string item_category` | GA4 `item_category` |
    | 20 | `string item_list_id` | GA4 `item_list_id` — **khác** `placement_id` (placement = slot render; list = ngữ cảnh danh sách) |
    | 21 | `string item_list_name` | |
    | 22 | `string event_group_id` | nối N row fan-out của cùng 1 logical event (D3) |
    | 23 | `string shipping_tier` | |
    | 24 | `string payment_type` | |
  - `position` **tái dùng** làm GA4 `index` (1-based) — không thêm field mới.
  - Cập nhật doc-comment: nêu rõ quy ước fan-out + `event_group_id`.
- Cập nhật `GetSellerFunnelResponse`: `+ int64 begin_checkouts = 5; + int64 purchases = 6;` (additive).
- OpenSpec: delta cho `openspec/specs/tracking/spec.md` (Requirement mới: "Ecommerce funnel context",
  "Multi-item events fan out to one row per item").

**Gate:** `buf lint` + `buf breaking` pass (Docker). `openspec validate <id> --strict`.

---

### W1 · Data Layer + Dispatcher — `team-frontend` ⭐ trái tim của pattern
**Mục tiêu:** UI không còn biết destination là ai.

Module mới `src/lib/analytics/`:

| File | Trách nhiệm |
|---|---|
| `schema.ts` | `EcommerceItem`, `EcommerceParams`, union `GA4EventName` |
| `dataLayer.ts` | `window.dataLayer` push; **reset `{ecommerce: null}` trước mỗi push** (best practice GA4 tránh merge rác); SSR-safe; never throws |
| `map.ts` | Bảng map 2 chiều GA4 name ⇄ internal `TrackEventType`. **Một chỗ duy nhất** (D2) |
| `queue.ts` | Ring buffer + flush khi: `>= 20` event **hoặc** idle `2s` **hoặc** `visibilitychange: hidden`. Gửi dạng **array** — collector đã hỗ trợ sẵn |
| `destinations/edge.ts` | Beacon tới `/api/track` (di chuyển logic từ `track.ts`), nay đi qua `queue` |
| `destinations/gtm.ts` | Chỉ load script GTM khi `NEXT_PUBLIC_GTM_ID` có giá trị |
| `destinations/index.ts` | Registry destination |
| `dispatcher.ts` | `trackEcommerce(name, params)` → push dataLayer **+** fan-out `items[]` thành N beacon chung `event_group_id` → đẩy vào queue |

Thay đổi khác:
- `src/lib/track.ts` → **shim deprecated**, delegate sang `dispatcher`. 6 call site cũ chạy nguyên.
  Không big-bang migration.
- `<AnalyticsProvider>` gắn vào `src/app/layout.tsx`: init dataLayer, load GTM có điều kiện,
  flush queue on unload.
- Migrate call site sang GA4 name:
  | Component | GA4 event |
  |---|---|
  | `TrackView` | `view_item` |
  | `TrackLink` | `select_item` |
  | `TrackImpression` / `SearchImpressions` | `view_item_list` — **1 request cho cả list** (fix G3) |
  | `AddToCartButton` | `add_to_cart` |
  | `CartView` | `view_cart`, `remove_from_cart` |
  | `CheckoutView` | `begin_checkout`, `add_shipping_info` |
  | `MockPaymentView` | `add_payment_info` |
  | trang order-success | `purchase` (có `transaction_id`) |
- `ListingCard` đã có `listing.price` / `stock` / `title` → đủ dữ liệu item-level, không cần fetch thêm.

**Gate:** `npm run build` + `tsc` clean. Unit test mới: shape dataLayer, reset `ecommerce:null`,
batching/flush theo 3 trigger, map round-trip, SSR no-op, never-throw.

---

### W2 · Edge collector — `team-gateway`
- `beaconEventTypes`: thêm 4 key mới **+ alias GA4 name** (`view_item`, `select_item`,
  `view_item_list`, …) → wire forgiving, FE cũ/mới đều nhận.
- `trackBeacon` struct + map sang `TrackingEvent`: 12 field ecommerce mới.
- Giữ nguyên tính atomic-reject của `parseBeacons` (validate hết rồi mới produce).
- Thêm cap số phần tử trong 1 batch (vd 100) bên cạnh `maxBeaconBytes` 64KB đang có.
- **Không** thêm logic ecommerce nào khác (Rule 2).

**Gate:** `go test ./...` + `go vet`. Test mới trong `collector_test.go`: batch có `event_group_id`,
field tiền round-trip, alias GA4 name map đúng, batch quá lớn bị từ chối.

---

### W3 · Warehouse + Query — `team-analytics`
- `warehouse.TrackingRecord` + `warehouse.Schema`: append 12 cột (append-only, parity test tự canh).
- **Migration (G6 — quan trọng):** thêm bước idempotent `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`
  cho cả DuckDB và BigQuery adapter. Hiện chỉ có `CREATE TABLE IF NOT EXISTS` → bảng cũ sẽ **không**
  có cột mới và insert sẽ vỡ. Cần test: mở lại file `.duckdb` schema cũ → migrate → insert thành công.
- `consumer/tracking.go`: map field mới; `eventTypeName` thêm 4 case.
- `query/duckdb.go`: funnel `+ begin_checkouts, purchases`; điền `GetSellerFunnelResponse` field 5,6.
- **View `ga4_events`**: đổi tên cột sang chuẩn GA4 (`event_type→event_name` qua map, `position→index`,
  `listing_id→item_id`, …) → consumer kiểu BigQuery/GA4 dùng được mà không đụng bảng gốc (D2).

**Gate:** `go test ./...`, parity test DuckDB↔BigQuery pass, test migration bảng cũ.

---

### W4 · Downstream — `platform-recsys`
- `recsys/weights.py` / config `event_weights`: thêm 4 event mới. Thứ tự confidence đề xuất:
  `purchase` > `begin_checkout` > `add_to_cart` > `add_payment_info` > `view_cart` > `view` > `click` > `impression`.
- `recsys/warehouse.py` `TRACKING_COLUMNS`: thêm cột mới — đã có cơ chế filter theo cột hiện diện
  (`present = [c for c in TRACKING_COLUMNS if c in df.columns]`) nên **backward compatible** với parquet cũ.
- `item_list_id` + `position(index)` mở đường debias vị trí cho ranker
  (nối với change `wire-debiased-ctr-ranker-features` đã có).
- `sample_data`: bổ sung cột mới vào generator.

**Gate:** `pytest`, `ruff`. Không đổi kết quả eval trên dữ liệu cũ (cột mới null → weight fallback).

---

### W5 · E2E — `platform-e2e` (chạy song song W1–W4)
- Mở rộng `tracking.feature`:
  - dataLayer chứa event GA4 kèm `ecommerce.items` (assert qua Playwright `page.evaluate`).
  - 1 danh sách kết quả ⇒ **1 request** tới `/api/track` (regression guard cho G3).
  - `purchase` có `transaction_id` → xuống warehouse; N item ⇒ N row cùng `event_group_id`.
  - GTM **không** được load khi `NEXT_PUBLIC_GTM_ID` rỗng (no external network).
- Cập nhật `FEATURES.yaml` của repo sở hữu; flip status → automated.

---

## 4. Thứ tự & song song hoá

```
W0 (contract)
 ├─► W1 (frontend)  ──┐
 ├─► W2 (gateway)   ──┼─► W5 (e2e, convergence gate)
 ├─► W3 (analytics) ──┤
 └─► W4 (recsys)    ──┘
```
W1–W4 độc lập nhau sau khi W0 merge (mỗi repo re-vendor `proto/` + `buf generate` trong cây của mình,
theo ADR-0001). → fan-out được cho nhiều session song song (`/pscrum` hoặc `/spec-dispatch`).

---

## 5. Rủi ro & cách chặn

| Rủi ro | Chặn bằng |
|---|---|
| Double-count doanh thu (tracking `purchase` vs `order_facts`) | Quy tắc §0.1 + `ga4_events` view **không** expose `value` như revenue; dashboard revenue chỉ đọc `order_facts` |
| Bảng warehouse cũ không có cột mới (G6) | Bước `ALTER TABLE ADD COLUMN IF NOT EXISTS` idempotent + test mở lại file schema cũ |
| Payload phình / lộ PII | Không gửi `item_name`; cap batch size + số phần tử ở edge |
| GTM gọi mạng ngoài trong CI/e2e | `NEXT_PUBLIC_GTM_ID` rỗng mặc định + e2e assert GTM không load |
| Breaking recsys/funnel khi đổi tên event | D2: enum nội bộ bất biến, GA4 name chỉ ở dataLayer + view |
| Regression "N request cho N item" quay lại | E2E guard đếm số request |
| Mất event khi GTM/SDK chưa sẵn sàng | `dataLayer` là Array queue + `queue.ts` tự drain |

---

## 6. Ngoài phạm vi (defer)

- Server-side GA4 qua **Measurement Protocol** (sGTM). Seam đã rõ: một worker sibling của
  `team-analytics` consume `analytics.events`. **Không** nhét vào `team-analytics` — service đó
  sở hữu warehouse, không phải fan-out destination.
- Meta CAPI / Ads destination.
- Consent Mode v2 (G7).
- Đổi `analytics.events` sang schema-registry / Avro.

---

## 7. Bước tiếp theo

1. Chạy `/opsx:propose` tạo OpenSpec change cho W0 (contract) trước — các wave sau phụ thuộc nó.
2. Sau khi W0 archived: fan-out W1–W4 bằng `/spec-dispatch` hoặc `/pscrum`.
3. Cuối cùng chạy `push-agora-snapshot.sh` đồng bộ sang `full_team_repo`.
