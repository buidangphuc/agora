# [W3-T1] Gateway forwarders cho saga (team-gateway)

## Role
SE

## Objective
Gateway route order/cart/address/payment RPC, verify JWT once → forward principal, gate scope.

## Write-set (EXCLUSIVE)
- team-gateway (edit — forwarder methods + env UPSTREAM_ORDER_ADDR/UPSTREAM_PAYMENT_ADDR + .env.example)

## Read-only dependencies
- platform-core/packages/proto; 00-spec.md §Architecture (auth verify-once, Rule 2)

## Contracts you consume
- order/cart/address/payment RPC (W0-T1) từ team-order/identity/payment

## Acceptance criteria
- [ ] Mọi RPC saga đi qua gateway; principal forward dạng x-principal-* (rebuilt mỗi hop)
- [ ] Scope buyer/seller đúng; anonymous → public scopes
- [ ] .env.example cập nhật (drift gate); gofmt/vet sạch

## Verify
docker run go test ./... trong team-gateway

## Out of scope
- KHÔNG làm real-time edge (wave7-task1) — cùng repo, khác wave; không thêm route showcase (wave9)
