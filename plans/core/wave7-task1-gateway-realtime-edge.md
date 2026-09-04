# [W7-T1] Real-time edge: Kafka → SSE/WS (team-gateway)

## Role
SE

## Objective
Xây 1 cầu push dùng chung: gateway consume Kafka, multiplex ra SSE/WS theo room. Flash-sale, chat,
notification bell, cockpit ticker đều dùng đường này. Transport-only, không business logic (Rule 2).

## Write-set (EXCLUSIVE)
- team-gateway (edit — SSE/WS hub + Kafka consumer bridge + route /stream, env KAFKA/ROOM config)

## Read-only dependencies
- platform-core/packages/proto; 00-spec.md §Architecture (1 real-time edge, Rule 2)
- Kafka topics: order.events, chat.events, ListingStockChanged

## Contracts you implement
- SSE/WS endpoint qua gateway, subscribe theo room: listing:{id}, chat:{thread}, user:{id}, ops:orders
- Auth: verify JWT once; chỉ đẩy message room mà principal được phép (user:{self}, v.v.)

## Acceptance criteria
- [ ] 1 client subscribe room nhận đúng message khi event bắn lên Kafka
- [ ] Fan-out nhiều client cùng room; đóng kết nối sạch (không rò goroutine)
- [ ] Không nhúng business logic (chỉ route/filter theo room + scope)

## Verify
docker run go test ./... + smoke: publish Kafka → client SSE nhận

## Out of scope
- KHÔNG làm forwarder RPC showcase (wave9-task1) — cùng repo, khác wave; không sửa infra
