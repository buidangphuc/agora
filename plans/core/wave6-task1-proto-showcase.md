# [W6-T1] Proto showcase — contracts cho 6 trụ cột (SOLO)

## Role
SA

## Objective
Additive contract cho các trụ showcase: ai endpoints, chat + chat.events, notification,
ListingStockChanged, order GetSagaState/ForceFail. Xong = Phase B code theo.

## Write-set (EXCLUSIVE)
- platform-core/packages/proto (edit — ai/chat/notification proto + mở rộng domain + order)

## Read-only dependencies
- platform-core/docs/ADR (0002 broker, 0004 observability); 00-spec.md §Contracts

## Contracts you implement
- ai: Assistant(query)→{reply,product_cards[]} streaming; MagicListing(title,image_key)→
  {description,suggested_price,category}; ChatCopilot(thread_ctx)→{suggestions[]}; SemanticSearch(query)→{listing_ids[]}
- chat: Message + SendMessage/GetThread/ListThreads; Kafka chat.events{MessageSent}
- notification: Notification + ListNotifications/MarkRead (consume order.events + chat.events)
- domain: Kafka ListingStockChanged{listing_id,sold,total} + cờ is_flash_sale
- order: GetSagaState(order_id)→{steps[]}; ForceFail(order_id) (test-only, guard scope/flag)

## Acceptance criteria
- [ ] `buf lint` + `buf breaking` pass (additive)
- [ ] Ghi chú re-vendor cho team-ai/chat/notification/domain/order

## Verify
docker run buf lint && buf breaking

## Out of scope
- Không implement service; không đụng infra (đó là wave7-task2)
