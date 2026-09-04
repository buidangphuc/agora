# [W8-T2] AI endpoints (team-ai)

## Role
DS-MLE

## Objective
team-ai expose 4 endpoint provider-agnostic: assistant (RAG+function-call→product cards),
magic-listing, chat-copilot, semantic-text-search. **Model/kết quả do team-ai tự lo**; MVP = JSON đúng schema (stub ok).

## Write-set (EXCLUSIVE)
- team-ai (edit — app/ endpoints + schema + indexer từ listing.events → Qdrant + tests)

## Read-only dependencies
- proto/HTTP schema (W6-T1); 00-spec.md §Contracts (ai); Kafka listing.events; Qdrant :6333

## Contracts you implement
- Assistant(query)→{reply, product_cards[]} (streaming); MagicListing(title,image_key)→
  {description,suggested_price,category}; ChatCopilot(thread_ctx)→{suggestions[]}; SemanticSearch(query)→{listing_ids[]}

## Acceptance criteria
- [ ] Mỗi endpoint trả JSON đúng schema (stub deterministic chấp nhận nếu chưa cắm model)
- [ ] Indexer consume listing.events → upsert/delete vector Qdrant; rebuild được từ replay
- [ ] pytest xanh; ≥1 golden test/endpoint

## Verify
pytest trong team-ai

## Out of scope
- KHÔNG chọn/khoá LLM provider trong plan (team-ai tự quyết); không visual search (hoãn); không route gateway
