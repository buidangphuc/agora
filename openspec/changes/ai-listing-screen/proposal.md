## Why
Q6 makes listing status server-owned: every content write in team-domain is auto-screened and a flagged listing waits in an admin queue. team-domain calls `AIService.ScreenListing` on team-ai for the second screening stage (after its local keyword list). team-ai has no such RPC, and its listing indexer still treats any status other than DRAFT/REJECTED as live, so `PENDING_REVIEW`, `BANNED` and `HELD` listings would be retrievable by the shopping assistant. This slice adds the screen verdict (mock by default, per the project LLM rule) and narrows the RAG index to `PUBLISHED` listings.

initiative: q6-trust-enforcement

## What changes
Repos: team-ai (code), platform-e2e (one edge scenario). Capability: listing screen verdict and published-only RAG index. Depends on `q6-trust-contract` (vendored `ai.proto` with `ScreenListing`, vendored `listing.proto` with the new `ListingStatus` values; run `make proto` in team-ai before coding).

- `AIService.ScreenListing` (gRPC, internal): scope `ai.classify`, service principals only, not routed by the gateway (the embedded `UnimplementedAIServiceHandler` answers 501, as for `ClassifyTags`). Returns `verdict` ALLOW or REVIEW, plain-slug `labels`, and `backend` (`mock` | `llm_router`).
- Backend switch `SCREEN_BACKEND: Literal["mock", "llm_router"] = "mock"` in `app/core/config/ai.py`, dispatched by an exhaustive `match` in a factory (mirror `ChatBackend` / `build_chat_service`). Mock is deterministic: REVIEW iff title or description contains the fixture marker `SCREEN_MOCK_MARKER` (case-insensitive; default `screen-review-marker`, which no existing e2e seed contains, so clean seed listings still auto-publish), else ALLOW; no network, no LLM. `llm_router` calls the LLM chain with a bounded timeout `SCREEN_TIMEOUT_SECONDS` (default 1.5 s, below team-domain's 2 s budget).
- Fail-safe contract with the caller: team-ai never answers ALLOW because something broke. A provider error or timeout is gRPC `UNAVAILABLE` (team-domain turns that into `PENDING_REVIEW`); unparseable model output is REVIEW with label `unparseable`.
- Labels are a closed set `prohibited_item | counterfeit | misleading | inappropriate | other` (mock label: `mock_marker`); anything else the model returns is dropped, so listing text cannot inject arbitrary flags. team-domain stores them as `ai:<label>`.
- `listing_indexer`: only `PUBLISHED` (and legacy `UNSPECIFIED`, kept as today) is indexed; `DRAFT`, `REJECTED`, `PENDING_REVIEW`, `BANNED`, `HELD` and any status value the stubs do not know delete the listing from RAG. `decode.py` maps the three new enum values, keeps UNSPECIFIED (0) as `""` (indexed) and decodes any other unknown number to `"UNKNOWN"` (deleted).
- Docs: `team-ai/README.md` (RPC list, `SCREEN_BACKEND` switch), `team-ai/.env.example`.

## Non-goals
- Image scanning; keyword screening (team-domain); the pending-review state and admin queue (domain-listing-moderation); a new scope in team-identity (`ai.classify` is reused); gateway code changes; using the screen anywhere except listing writes.
- Calibrating the LLM prompt against real listings (needs a labelled set; mock is the only backend exercised by CI and e2e).

## Non-functional requirements
- Latency: mock verdict p99 < 10 ms in-process; `llm_router` bounded by `SCREEN_TIMEOUT_SECONDS` (1.5 s) so the caller's 2 s budget holds.
- Availability: stateless; an outage degrades to slower publishing (caller fails safe), never to unscreened publishing.
- Limits: title and description sent to the LLM are truncated to 8000 characters each; the mock scans the full text. Request size is already capped by the gRPC server.
- Default posture: `SCREEN_BACKEND=mock` in every environment unless explicitly set; e2e and CI never reach an LLM.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `title`, `description`, `category_id` of a listing in `ScreenListingRequest` | internal (public once published; not personal by design, may incidentally contain a seller's phone number or address) | decide ALLOW / REVIEW | platform duty to act against prohibited goods (Decree 85/2021) | not stored; not logged; passed to the LLM provider only when `SCREEN_BACKEND=llm_router` |
| `listing_id` | internal identifier | log correlation | same | logged with verdict, backend and latency; empty on create (team-domain has no id yet); follows existing team-ai log retention, no new rule |
| `verdict`, `labels`, `backend` | internal | outcome | same | not stored in team-ai; the caller stores flags |

The mock backend sends nothing outside the cluster. Langfuse tracing of the `llm_router` call (when enabled) records the prompt like other LLM calls; listing text is treated as internal data there. DPO sign-off on Langfuse traces when a real LLM is on is a NEEDS_HUMAN item before release (initiative clarifications); the mock default sends nothing.

## Threat model
| asset | threat | mitigation |
|---|---|---|
| Listing integrity (no prohibited item published unreviewed) | seller crafts text to talk the model into ALLOW (prompt injection) | text is passed as delimited data, output must be strict JSON from a closed schema; the keyword list and admin queue stay independent layers; REVIEW is the default on any parse failure |
| Same | attacker forces ALLOW by making team-ai fail | failure is `UNAVAILABLE`, never ALLOW; the caller maps it to `PENDING_REVIEW` |
| Same | a browser or user token calls the screen to probe the classifier or flood the LLM | scope `ai.classify` and `principal.type == "service"` both required; gateway answers 501 |
| LLM budget | repeated screening of one listing | bounded timeout, per-principal rate limit unchanged; one call per content write |
| Seller data | listing text leaks via logs | title and description are never logged; only `listing_id`, verdict, backend |
| RAG index | a hidden or banned listing stays retrievable | indexer keeps only PUBLISHED; every status change already emits `ListingChanged` |

Authz rule: only a service principal holding `ai.classify` may call `ScreenListing`.
