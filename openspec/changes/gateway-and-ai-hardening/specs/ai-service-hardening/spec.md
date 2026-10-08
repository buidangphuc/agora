## Purpose

Defines who may call each `team-ai` AI RPC (`platform.ai.v1.AIService` and `platform.chat.v1.ChatService/StreamChat`),
what error information callers receive, how LLM quota reservations are keyed, and what `LLM_TRACE_CONTENT`
guarantees about prompt and completion text sent to Langfuse. Authentication stays at the gateway (ADR-0003/0006);
`team-ai` only enforces scopes on the principal the gateway forwards.

## ADDED Requirements

### Requirement: Seller features require the listing write scope

`MagicListing` and `ChatCopilot` SHALL succeed only for a principal whose scopes include `listing.write`. Any other
principal, including anonymous and buyers, SHALL receive `PERMISSION_DENIED` with the stable
`insufficient_scope` message, and no assistant logic SHALL run. When `ChatCopilot` is called with a non-empty
`seller_id` that differs from the caller's principal id, the call SHALL be denied unless the principal also holds
the `admin` scope.

#### Scenario: Seller generates a listing draft

- **WHEN** a principal with `listing.write` calls `MagicListing` with a valid title hint
- **THEN** the call succeeds with the generated fields

#### Scenario: Buyer is denied seller features

- **WHEN** a principal without `listing.write` (a buyer) calls `MagicListing` or `ChatCopilot`
- **THEN** the call fails with `PERMISSION_DENIED` and `insufficient_scope`, and the assistant is not invoked

#### Scenario: Anonymous caller is denied seller features

- **WHEN** an anonymous principal calls `ChatCopilot`
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: A seller cannot act as another seller

- **WHEN** a seller with `listing.write` calls `ChatCopilot` with another seller's id in `seller_id`
- **THEN** the call fails with `PERMISSION_DENIED`, while the same call with the caller's own id succeeds

### Requirement: Shopper AI features require an authenticated user scope

`ShoppingAssistant` and `StreamChat` SHALL succeed only for a principal whose scopes include `ai:use`. Anonymous
callers (whose scopes are the public scopes) SHALL receive `PERMISSION_DENIED` before any model call, quota
reservation or session access. `team-identity` SHALL grant `ai:use` to the buyer, seller and admin roles and SHALL
NOT include it in any anonymous or public scope set. `SummarizeReviews`, which is stateless and rendered on the
public product page, SHALL require only `listing.read`.

#### Scenario: Logged-in buyer can chat

- **WHEN** a buyer whose token carries `ai:use` calls `StreamChat`
- **THEN** the stream delivers reply chunks and a terminal `done` message

#### Scenario: Anonymous caller cannot reach the model

- **WHEN** an anonymous principal calls `StreamChat` or `ShoppingAssistant`
- **THEN** the call fails with `PERMISSION_DENIED`, no LLM request is made, no quota is reserved and no session
  history is read or written

#### Scenario: Identity grants the scope to every signed-in role

- **WHEN** a user registers or logs in as buyer, seller or admin
- **THEN** the issued token's scopes include `ai:use`, and the anonymous public scope set does not

#### Scenario: Review summaries stay public

- **WHEN** an anonymous principal (scopes `listing.read`, `search:read`) calls `SummarizeReviews`
- **THEN** the call succeeds

### Requirement: Callers never see internal exception text

For every `AIService` RPC and `StreamChat`, an invalid request SHALL fail with `INVALID_ARGUMENT` whose message
names only the offending field(s), never the submitted values; a known "service not available" condition SHALL
fail with `UNAVAILABLE`; any other unexpected failure SHALL fail with `INTERNAL` and the fixed message
`internal error`. The full exception SHALL be logged server-side with the request id, and the status message SHALL
NOT contain exception text, stack frames, file paths or user-supplied content.

#### Scenario: Validation error does not echo input

- **WHEN** `MagicListing` is called with a one-character title hint (below the minimum length)
- **THEN** the call fails with `INVALID_ARGUMENT`, the message names `title_hint`, and it does not contain the
  submitted text

#### Scenario: Unexpected failure is opaque

- **WHEN** the assistant raises an unexpected exception whose text contains a secret-looking string
- **THEN** the call fails with `INTERNAL` and message `internal error`, the secret string is absent from the status
  message and trailing metadata, and the log line for the request id contains the exception

#### Scenario: Unavailability is distinguishable

- **WHEN** the assistant reports a service-unavailable condition
- **THEN** the call fails with `UNAVAILABLE`, not `INTERNAL`

### Requirement: Quota reservations are keyed by a server-minted call id

The LLM quota reservation for a `StreamChat` call SHALL use an idempotency key minted by `team-ai` for that call.
It SHALL NOT use the request id or any other client-influenced value, so two separate calls are always charged
separately even when they carry the same `X-Request-Id`.

#### Scenario: Reusing a request id does not skip the charge

- **WHEN** one principal makes two sequential `StreamChat` calls with the same `x-request-id` while quota is enabled
- **THEN** quota usage increases by the cost of both calls

#### Scenario: Failure still refunds only its own reservation

- **WHEN** one of two concurrent calls fails before its first chunk
- **THEN** only that call's reservation is refunded

### Requirement: LLM_TRACE_CONTENT governs every sink, including Langfuse

The setting `LLM_TRACE_CONTENT` (`off`, `redacted`, `full`) SHALL define one content policy for all places prompt and
completion text can be recorded: process logs and Langfuse traces. With `off`, no prompt or completion text SHALL be
sent to Langfuse (trace structure, model, latency, usage and error status still are), and log lines SHALL contain a
placeholder instead of user text. With `redacted` (default), text sent to Langfuse SHALL have the same masking as
text sent to the model (email, secrets, Vietnamese phone numbers, keyword-anchored national ids). With `full`,
text passes through unchanged. The text given to the model itself SHALL stay masked in `off` and `redacted`.

#### Scenario: off sends no content to Langfuse

- **WHEN** a chat request runs with `LLM_TRACE_CONTENT=off` and Langfuse enabled
- **THEN** no exported trace or observation contains the prompt or the reply text, while model name, token usage and
  latency are present

#### Scenario: redacted masks what Langfuse receives

- **WHEN** a chat request whose text contains a Vietnamese mobile number runs with `LLM_TRACE_CONTENT=redacted`
- **THEN** the exported trace input shows `[phone]` and never the digits

#### Scenario: full keeps content

- **WHEN** the same request runs with `LLM_TRACE_CONTENT=full`
- **THEN** the exported trace input equals the submitted text

#### Scenario: Assistant logs follow the same policy

- **WHEN** `ShoppingAssistant` logs a query under `LLM_TRACE_CONTENT=off`
- **THEN** the log line contains a placeholder and not the message
