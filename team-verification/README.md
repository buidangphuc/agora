# team-verification — Seller/User KYC Verification (Go, mock)

Dịch vụ `team-verification` quản lý luồng xác minh danh tính (KYC) dạng **mock**:
người dùng nộp tham chiếu tài liệu (`SubmitKyc` → trạng thái `PENDING`), quản trị
viên duyệt/từ chối (`ReviewKyc` → `VERIFIED` / `REJECTED`), và giao diện tra cứu
trạng thái + huy hiệu đã xác minh (`GetVerificationStatus` → `{status, badge}`).

Tuân thủ **Database-per-service**, sở hữu database `verification_db` riêng và cung
cấp gRPC service `platform.verification.v1.VerificationService` trên port `:50062`.

> MOCK: chỉ lưu **tham chiếu tài liệu** (chuỗi/khoá lưu trữ), không lưu tài liệu thật.

## Cấu trúc

```
team-verification/
├── cmd/server/main.go                 # Entrypoint gRPC + graceful shutdown
├── generated/platform/                # Proto stubs (buf generate)
│   ├── common/v1/
│   └── verification/v1/
├── internal/
│   ├── config/                        # Nạp cấu hình từ ENV
│   ├── grpcserver/                    # Cấu hình gRPC server
│   ├── handler/                       # gRPC handler (VerificationService)
│   ├── service/                       # Use case: submit / review / status
│   ├── repository/                    # Interface + Postgres & InMemory impl
│   └── interceptor/                   # Resolve principal từ metadata (auth-scope)
├── migrations/                        # kyc_submissions schema
└── proto/                             # Proto vendored từ platform-core
```

## API gRPC (`VerificationService`)

| RPC | Request | Response | Mô tả |
|---|---|---|---|
| `SubmitKyc` | `doc_type`, `doc_ref` | `id`, `status` | Nộp tài liệu (mock) → tạo bản ghi `PENDING`. |
| `GetVerificationStatus` | `user_id` | `status`, `badge` | Trạng thái hiện tại; `badge = (status == VERIFIED)`. |
| `ReviewKyc` | `id`, `decision` | `status` | Quản trị viên duyệt (`approve`) / từ chối (`reject`). |

## Data model — `kyc_submissions`

`id, user_id, doc_type, doc_ref, status, reviewed_at, created_at`;
`status ∈ {PENDING, VERIFIED, REJECTED}`.

## Chạy & kiểm thử

```bash
export PATH=/opt/homebrew/bin:$PATH
buf generate        # regen stubs (hoặc: make proto)
go test ./...       # unit tests (InMemory repo, không cần DB)
go run ./cmd/server # chạy service (fallback InMemory khi thiếu Postgres)
```

## Biến môi trường

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `GRPC_PORT` | `50062` | Cổng gRPC |
| `DATABASE_URL` | `postgres://verification_svc:verification_pass@localhost:5444/verification_db?sslmode=disable` | Postgres (`verification_db`) |
