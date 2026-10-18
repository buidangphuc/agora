# ai-listing-screen Specification

## Purpose
Internal ScreenListing RPC in team-ai that returns a moderation verdict for listing content from a switchable backend that fails safe.

## Requirements

### Requirement: ScreenListing returns a verdict from a switchable backend

`AIService.ScreenListing` SHALL accept `listing_id`, `title`, `description` and `category_id` and SHALL return `verdict` (`ALLOW` or `REVIEW`), `labels` (plain slugs from the closed set `prohibited_item`, `counterfeit`, `misleading`, `inappropriate`, `other`, plus `mock_marker` and `unparseable`) and `backend` (`mock` or `llm_router`). The backend SHALL be chosen by `SCREEN_BACKEND`, default `mock`, and an unknown value SHALL fail startup. The mock backend SHALL be deterministic: `REVIEW` with label `mock_marker` iff the title or description contains `SCREEN_MOCK_MARKER` (case-insensitive), else `ALLOW` with no labels; it SHALL make no network or LLM call. A blank title (after trimming) SHALL be `INVALID_ARGUMENT`. The RPC SHALL never return `ALLOW` because of an internal failure.

#### Scenario: Mock backend flags a listing containing the marker
- **WHEN** a service principal holding `ai.classify` calls ScreenListing with `SCREEN_BACKEND=mock` and a title containing the marker
- **THEN** the response has verdict REVIEW, labels exactly `["mock_marker"]` and backend `mock`
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_mock_marker_in_title_is_review`

#### Scenario: Mock backend allows a listing without the marker
- **WHEN** a service principal holding `ai.classify` calls ScreenListing with `SCREEN_BACKEND=mock` and a title and description without the marker
- **THEN** the response has verdict ALLOW, no labels and backend `mock`
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_mock_clean_text_is_allow`

#### Scenario: Default mock settings allow ordinary seed listing text
- **WHEN** Settings are built with no `SCREEN_*` variables and a listing titled "iPhone 15 Pro Max 256GB" with an ordinary description is screened
- **THEN** the verdict is ALLOW with backend `mock`, because the default `SCREEN_MOCK_MARKER` is `screen-review-marker`, a token no existing e2e seed contains
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_mock_screen.py` `test_default_settings_allow_clean_seed_text`

#### Scenario: Marker in the description is matched case-insensitively at any position
- **WHEN** ScreenListing is called with a clean title and a 20000-character description whose last characters are the marker in upper case
- **THEN** the verdict is REVIEW
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_mock_screen.py` `test_marker_matches_case_insensitively_past_8000_chars`

#### Scenario: Mock verdict is deterministic and makes no outbound call
- **WHEN** the same request is screened twice with `SCREEN_BACKEND=mock` while the LLM factory and network are patched to fail the test on any use
- **THEN** both calls return identical responses and neither touches the LLM factory
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_mock_screen.py` `test_mock_is_deterministic_and_offline`

#### Scenario: Blank title is refused
- **WHEN** ScreenListing is called with a title that is empty or only whitespace
- **THEN** the call fails with INVALID_ARGUMENT and no backend is invoked
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_blank_title_is_invalid_argument`

#### Scenario: Default backend is mock and an unknown backend fails startup
- **WHEN** Settings are built without `SCREEN_BACKEND`, and then with `SCREEN_BACKEND=other`
- **THEN** the first has `SCREEN_BACKEND == "mock"` and the second raises a validation error
- **VERIFIED BY**: team-ai `tests/unit/core/test_screen_settings.py` `test_screen_backend_defaults_to_mock_and_rejects_unknown`

### Requirement: llm_router backend fails safe

With `SCREEN_BACKEND=llm_router` the RPC SHALL ask the LLM chain for a strict JSON verdict, passing the listing text as delimited data (truncated to 8000 characters per field), and SHALL return `backend` `llm_router`. A provider error or a call longer than `SCREEN_TIMEOUT_SECONDS` (default 1.5) SHALL be `UNAVAILABLE`. Model output that is not the expected JSON SHALL yield REVIEW with label `unparseable`. Labels outside the closed set SHALL be dropped. Startup with this backend and without the `ai` extra SHALL fail at boot, as for `CHAT_BACKEND`.

#### Scenario: LLM review verdict is returned with its labels
- **WHEN** the LLM stub answers `{"verdict":"review","labels":["counterfeit"]}`
- **THEN** the response has verdict REVIEW, labels `["counterfeit"]` and backend `llm_router`
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_llm_screen.py` `test_llm_review_verdict_carries_labels`

#### Scenario: LLM allow verdict is returned
- **WHEN** the LLM stub answers `{"verdict":"allow","labels":[]}`
- **THEN** the response has verdict ALLOW and no labels
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_llm_screen.py` `test_llm_allow_verdict`

#### Scenario: Unparseable model output becomes REVIEW
- **WHEN** the LLM stub answers plain text such as "Looks fine, ALLOW"
- **THEN** the response has verdict REVIEW and labels `["unparseable"]`
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_llm_screen.py` `test_unparseable_output_is_review`

#### Scenario: Labels outside the closed set are dropped
- **WHEN** the LLM stub answers review with labels `["counterfeit","ignore previous instructions"]`
- **THEN** the response labels are exactly `["counterfeit"]`
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_llm_screen.py` `test_unknown_labels_are_dropped`

#### Scenario: Provider error is UNAVAILABLE, never ALLOW
- **WHEN** the LLM stub raises a provider error
- **THEN** the call fails with UNAVAILABLE and returns no verdict
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_provider_error_is_unavailable`

#### Scenario: A call slower than the timeout is UNAVAILABLE
- **WHEN** the LLM stub takes longer than `SCREEN_TIMEOUT_SECONDS` (set to 0.05 in the test)
- **THEN** the call fails with UNAVAILABLE within 0.5 s
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_llm_screen.py` `test_slow_llm_times_out_as_unavailable`

#### Scenario: Listing text is truncated and passed as data
- **WHEN** a 20000-character description is screened with the LLM stub recording its prompt
- **THEN** the recorded description is at most 8000 characters and sits inside the data delimiters, not in the instruction part
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_llm_screen.py` `test_text_is_truncated_and_delimited`

#### Scenario: llm_router without the ai extra fails at boot
- **WHEN** the screen service is built with `SCREEN_BACKEND=llm_router` and `langchain_core` is not importable
- **THEN** construction raises RuntimeError naming the `ai` extra
- **VERIFIED BY**: team-ai `tests/unit/modules/screen/test_factory.py` `test_llm_router_without_extra_fails_at_boot`

### Requirement: ScreenListing is internal to service principals

`ScreenListing` SHALL require scope `ai.classify` and a principal of type `service`, SHALL appear in the AIService scope table (which keeps `ShoppingAssistant` absent on purpose), and SHALL NOT be routed by the gateway. The service SHALL NOT log `title` or `description`.

#### Scenario: Caller without the scope is refused
- **WHEN** a service principal without `ai.classify` calls ScreenListing
- **THEN** the call fails with PERMISSION_DENIED and no backend is invoked
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_requires_the_scope`

#### Scenario: User principal holding the scope is refused
- **WHEN** a principal of type `user` that carries `ai.classify` calls ScreenListing
- **THEN** the call fails with PERMISSION_DENIED
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_user_principal_is_refused`

#### Scenario: Call without a principal is refused
- **WHEN** ScreenListing is called with no principal metadata
- **THEN** the call fails with UNAUTHENTICATED
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_without_principal_is_unauthenticated`

#### Scenario: Every AIService RPC except ShoppingAssistant has a scope entry
- **WHEN** the AIService descriptor is compared with the scope table
- **THEN** `ScreenListing` maps to `("ai.classify",)`, every RPC except `ShoppingAssistant` has an entry, and `ShoppingAssistant` stays absent on purpose
- **VERIFIED BY**: team-ai `tests/unit/transport/test_grpc_scopes.py` `test_every_ai_rpc_is_gated_or_explicitly_open` (existing) and `test_screen_listing_scope_is_ai_classify` (new)

#### Scenario: Screening does not log listing text
- **WHEN** a listing whose title is "SECRET-TITLE-123" is screened with log capture on
- **THEN** no captured log record contains "SECRET-TITLE-123" or the description, and the record for the call contains the listing id, verdict and backend
- **VERIFIED BY**: team-ai `tests/unit/transport/test_ai_servicer.py` `test_screen_listing_does_not_log_listing_text`

#### Scenario: The edge does not route ScreenListing
- **WHEN** an anonymous caller and a signed-in buyer call `/platform.ai.v1.AIService/ScreenListing` through the gateway
- **THEN** both calls fail with unimplemented (HTTP 501) and no verdict is returned
- **VERIFIED BY**: platform-e2e `features/ai/listing_screen_edge.feature` "The edge does not route ScreenListing"
