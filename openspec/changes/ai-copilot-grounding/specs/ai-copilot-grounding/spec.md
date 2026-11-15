## ADDED Requirements

### Requirement: Copilot drafts are grounded in the listing stock
`ChatCopilot` SHALL read the discussed listing through `ListingService.GetListing(listing_id)` and SHALL NOT return any draft that claims the item is in stock unless the listing is published, owned by the effective seller and has stock above 0 (listing `stock`, or the sum of variant stock when the listing has variants).

#### Scenario: Out-of-stock listing
- **WHEN** a seller calls `ChatCopilot` with buyer message "còn hàng không?" for a published own listing whose stock is 0
- **THEN** none of the returned drafts contains a phrase claiming availability ("còn hàng", "còn sẵn", "có sẵn" without a negation) and at least one draft tells the buyer the item is out of stock
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ai/chat_copilot.feature` `Copilot does not claim stock for an out-of-stock listing`

#### Scenario: In-stock listing
- **WHEN** a seller calls `ChatCopilot` with "còn hàng không?" for a published own listing whose stock is 12
- **THEN** the response has 3 drafts and each confirms the item is in stock without stating a quantity above 12
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_in_stock_listing_confirms_stock`

#### Scenario: Copilot stock question for an unknown listing
- **WHEN** a seller calls `ChatCopilot` with a stock question and a `listing_id` that team-domain answers with `NOT_FOUND`
- **THEN** no returned draft makes a stock claim and the call succeeds with 3 drafts
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_unknown_listing_makes_no_stock_claim`

#### Scenario: Listing lookup fails
- **WHEN** a seller calls `ChatCopilot` with a stock question while the `GetListing` call times out after 200 ms
- **THEN** no returned draft makes a stock claim and the response arrives within 500 ms with status OK
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_lookup_timeout_degrades_without_claim`

#### Scenario: No listing id given
- **WHEN** a seller calls `ChatCopilot` with a stock question and an empty `listing_id`
- **THEN** no returned draft makes a stock claim and no `GetListing` call is made
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_empty_listing_id_skips_lookup`

#### Scenario: Unpublished listing
- **WHEN** a seller asks a stock question about their own listing whose status is `DRAFT` or `HELD`
- **THEN** no returned draft makes a stock claim
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_unpublished_listing_makes_no_stock_claim`

#### Scenario: Listing owned by another seller
- **WHEN** seller A calls `ChatCopilot` with a `listing_id` owned by seller B, with stock 5
- **THEN** no returned draft makes a stock claim, no field of B's listing appears in the response, and no field of it is put in the LLM prompt
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ai/chat_copilot.feature` `Copilot ignores a listing owned by another seller`

#### Scenario: Stock boundary of one
- **WHEN** a seller asks a stock question about a published own listing whose stock is exactly 1
- **THEN** the drafts confirm the item is in stock and none says it is out of stock
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_stock_of_one_is_in_stock`

### Requirement: Copilot makes no unverified delivery promise
No draft returned by `ChatCopilot` SHALL state a delivery duration, a dispatch deadline or a same-day dispatch, because team-ai holds no shipping data for the listing.

#### Scenario: Delivery-time question
- **WHEN** a seller calls `ChatCopilot` with "giao hàng bao lâu vậy shop?"
- **THEN** no returned draft matches a duration, a cut-off hour or "trong ngày" / "hỏa tốc" / "ngay hôm nay", and at least one draft asks the buyer for the delivery address
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ai/chat_copilot.feature` `Copilot makes no promise on a delivery-time question`

#### Scenario: Default drafts carry no availability claim
- **WHEN** a seller calls `ChatCopilot` with "shop ơi" for a listing whose stock is 0
- **THEN** no returned draft claims availability
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_default_drafts_have_no_availability_claim`

#### Scenario: Delivery question with a failing lookup
- **WHEN** a seller asks "giao hàng bao lâu vậy shop?" while the `GetListing` call fails with `UNAVAILABLE`
- **THEN** the call succeeds and no returned draft states a delivery duration
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_delivery_question_without_lookup_has_no_duration`

### Requirement: Copilot backend is selectable and defaults to mock
team-ai SHALL read `COPILOT_BACKEND` with values `mock` and `llm_router`, default `mock`. `mock` SHALL be a deterministic template over the retrieved listing facts and SHALL make no LLM call. `llm_router` SHALL send the redacted message and the facts through the existing LLM chain, SHALL validate the output against the facts, and SHALL fall back to the mock template when the call times out, fails or the output contradicts the facts.

#### Scenario: Mock is the default and is deterministic
- **WHEN** team-ai boots with no `COPILOT_BACKEND` and two identical `ChatCopilot` calls are made
- **THEN** both responses carry the same 3 drafts and no LLM call is recorded
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_backend.py` `test_mock_default_is_deterministic`

#### Scenario: Unknown backend value is rejected at boot
- **WHEN** team-ai boots with `COPILOT_BACKEND=gpt`
- **THEN** settings validation fails and the process exits before serving
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_backend.py` `test_unknown_backend_rejected`

#### Scenario: LLM output that contradicts the facts is replaced
- **WHEN** `COPILOT_BACKEND=llm_router` and the LLM returns a draft saying "còn hàng" for a listing with stock 0
- **THEN** the response carries the mock template drafts, none claims availability, and `model_version` is `rules-v1`
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_backend.py` `test_contradicting_llm_output_falls_back`

#### Scenario: LLM timeout falls back
- **WHEN** `COPILOT_BACKEND=llm_router` and the LLM call exceeds `COPILOT_TIMEOUT_SECONDS`
- **THEN** the call returns OK with the mock template drafts within `COPILOT_TIMEOUT_SECONDS` plus 200 ms
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_backend.py` `test_llm_timeout_falls_back`

#### Scenario: Prompt carries no ids
- **WHEN** `COPILOT_BACKEND=llm_router` and a copilot call is made with a seller id and a buyer message containing a phone number
- **THEN** the prompt sent to the LLM contains neither the seller id nor the phone number
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_backend.py` `test_prompt_has_no_ids_or_pii`

#### Scenario: Copilot latency with the mock backend
- **WHEN** 200 sequential `ChatCopilot` calls run with `COPILOT_BACKEND=mock` against a local team-domain
- **THEN** the p95 latency is at most 300 ms
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ai/chat_copilot.feature` `Copilot latency with the mock backend stays within budget`

### Requirement: Copilot keeps its caller authorisation
`ChatCopilot` SHALL keep rejecting a `seller_id` that differs from the caller, unless the caller is admin, before any listing lookup.

#### Scenario: Seller impersonation is denied
- **WHEN** seller A calls `ChatCopilot` with `seller_id` of seller B
- **THEN** the call fails with `PERMISSION_DENIED` and no `GetListing` call is made
- **VERIFIED BY**: `team-ai/tests/unit/transport/test_ai_copilot_authz.py` `test_other_seller_id_denied_before_lookup`

#### Scenario: Caller without the seller scope is refused
- **WHEN** a buyer principal calls `ChatCopilot`
- **THEN** the call fails with `PERMISSION_DENIED`
- **VERIFIED BY**: `team-ai/tests/unit/transport/test_ai_copilot_authz.py` `test_buyer_scope_denied`

### Requirement: Copilot, Magic Listing and review summary responses carry request_id and model_version
`ChatCopilot`, `MagicListing` and `SummarizeReviews` SHALL set `request_id` to a UUID hex minted per call and `model_version` to the version that produced the output: `rules-v1` for rule output and `llm_router:<prompt_version>` for LLM output.

#### Scenario: Copilot response carries provenance
- **WHEN** a seller calls `ChatCopilot` with the mock backend
- **THEN** the response has a 32-character hex `request_id` and `model_version` equal to `rules-v1`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ai/chat_copilot.feature` `Copilot response carries provenance`

#### Scenario: Copilot LLM output names the prompt version
- **WHEN** `COPILOT_BACKEND=llm_router` and a valid LLM draft set is returned
- **THEN** `model_version` equals `llm_router:` followed by the copilot prompt version
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_backend.py` `test_llm_model_version_names_prompt`

#### Scenario: Magic Listing response carries provenance
- **WHEN** a seller calls `MagicListing` with a title and category
- **THEN** the response has a 32-character hex `request_id` and `model_version` equal to `rules-v1`, and the generated fields are unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/ai/magic_listing.feature` `Magic Listing response carries provenance`

#### Scenario: Review summary carries provenance
- **WHEN** a buyer calls `SummarizeReviews` with 3 reviews
- **THEN** the response has a 32-character hex `request_id` and `model_version` equal to `rules-v1`
- **VERIFIED BY**: `team-ai/tests/unit/transport/test_ai_provenance.py` `test_summarize_reviews_has_provenance`

#### Scenario: Copilot request ids are unique per call
- **WHEN** the same `ChatCopilot` request is sent twice
- **THEN** the two responses carry different `request_id` values
- **VERIFIED BY**: `team-ai/tests/unit/transport/test_ai_provenance.py` `test_request_id_unique_per_call`

#### Scenario: Failed call returns no provenance
- **WHEN** `ChatCopilot` fails with `INVALID_ARGUMENT` because `buyer_message` is empty
- **THEN** the call returns the error status and no response body
- **VERIFIED BY**: `team-ai/tests/unit/transport/test_ai_provenance.py` `test_error_has_no_response_body`

### Requirement: Copilot logs carry no raw ids
team-ai SHALL NOT write a raw account id or unredacted message text to any log line produced by `ChatCopilot`, including the listing-lookup failure path.

#### Scenario: Copilot logs are redacted
- **WHEN** a seller calls `ChatCopilot` with a message containing a phone number and the listing lookup fails
- **THEN** the captured log output contains neither the seller id, the phone number nor the listing id
- **VERIFIED BY**: `team-ai/tests/unit/modules/business/ai_assistant/test_copilot_grounding.py` `test_logs_carry_no_raw_ids`

### Requirement: Copilot groundedness is gated by an eval
team-ai SHALL ship a copilot groundedness eval set where every case pins listing facts and asserts that no draft contradicts them, and `run_eval --min-score 0.8` SHALL fail below that score.

#### Scenario: Eval passes on the grounded copilot
- **WHEN** `run_eval --min-score 0.8` runs the copilot groundedness set with `COPILOT_BACKEND=mock`
- **THEN** the score is 1.0 and the command exits 0
- **VERIFIED BY**: `team-ai/tests/unit/test_run_eval_copilot.py` `test_copilot_groundedness_set_passes`

#### Scenario: Eval fails on a stock-contradicting draft
- **WHEN** the eval runs against a copilot stub that answers "còn hàng" for a stock-0 case
- **THEN** the score is below 0.8 and the command exits non-zero
- **VERIFIED BY**: `team-ai/tests/unit/test_run_eval_copilot.py` `test_copilot_groundedness_set_fails_on_contradiction`
