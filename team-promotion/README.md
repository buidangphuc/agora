# team-promotion — Go Voucher & Flash-Sale Microservice

Microservice quản lý **Voucher** (shop/platform) và **Flash-Sale** (khuyến mãi giới
hạn thời gian) trong kiến trúc Polyrepo E-Commerce. Dịch vụ chỉ **tính toán giảm
giá** — không bao giờ dịch chuyển tiền (payment/wallet vẫn là mock, xem AGENTS.md §7).

Chạy trên Go 1.22, cung cấp gRPC API (`:50061`), sở hữu DB riêng `promotion_db`
(PostgreSQL), phát sự kiện thay đổi trạng thái trên Kafka topic `promotion.events`
(ADR-0002), và tuân thủ nguyên tắc DB-per-service + Zero-Trust Principal Forwarding.

> Trạng thái hiện tại: **skeleton** — hai gRPC service đã đăng ký với handler
> `Unimplemented` (trả về `codes.Unimplemented`). Nghiệp vụ voucher/flash-sale sẽ
> được bổ sung ở Wave 1 (xem `plan-ecommerce/wave1-task1-promotion-service.md`).

## gRPC Services

- **`platform.promotion.v1.VoucherService`** — CRUD voucher; seam redemption tại
  checkout (`ValidateAndReserve` → `CommitReservation`/`ReleaseReservation`),
  idempotent theo `reservation_id` (mirror ReserveStock, ADR-0008).
- **`platform.promotion.v1.FlashSaleService`** — CRUD campaign; tra cứu flash-sale
  đang hoạt động theo listing + tồn kho còn lại (live stock).

## Cấu trúc

```
cmd/server/main.go            # DI wiring (config → resources → repos → services → handlers → grpc)
internal/config               # env-tagged Settings + DeclaredEnvKeys drift gate
internal/bootstrap            # pgxpool + feature flags + kafka producer lifecycle
internal/grpcserver           # gRPC server build + service registration
internal/handler              # VoucherHandler, FlashSaleHandler (Unimplemented)
internal/interceptor          # request-id + principal (zero-trust) interceptors
internal/service              # VoucherService, FlashSaleService (stubs)
internal/repository           # Postgres + in-memory repositories (stubs)
internal/featureflags         # OpenFeature + Flipt provider
migrations                    # 0001_promotion.{up,down}.sql
proto/                        # vendored copy of platform/promotion + platform/common
```

## Local

```bash
cp .env.example .env
buf generate            # → generated/ (gitignored)
go build ./...
```
