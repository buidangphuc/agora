# [W8-T3] Chat service (team-chat)

## Role
SE

## Objective
team-chat lưu message + lịch sử + presence cơ bản; emit chat.events; live delivery đi qua real-time edge (W7-T1). + OTel spans.

## Write-set (EXCLUSIVE)
- team-chat (edit — message handler/service/repo + migration messages/threads + Kafka producer + spans)

## Read-only dependencies
- proto chat (W6-T1); real-time edge (W7-T1, đã merge); 00-spec.md §Contracts

## Contracts you implement
- Message{id,thread_id,sender,body,ts}; RPC SendMessage/GetThread/ListThreads
- Kafka chat.events{MessageSent} (key = thread_id, room chat:{thread})

## Acceptance criteria
- [ ] SendMessage lưu DB + emit chat.events; 2 client cùng thread nhận realtime qua edge
- [ ] Reload thấy lịch sử; gửi idempotent (client_msg_id chống trùng)
- [ ] ≥3 test (send, history, idempotent)

## Verify
docker run go test ./... trong team-chat

## Out of scope
- Không làm AI copilot (đó là team-ai W8-T2); không SSE riêng (dùng edge); không route gateway (wave9)
