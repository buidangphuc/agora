## Context

`Storefront` (team-domain, `listing.v1`) is the seller's public shop page: one row per seller, owner-scoped
upsert, public `GetStorefront`. It has no name. Cart items come from team-order (`CartItem.seller_id`),
followed sellers from team-engagement (`seller_ids`), and the frontend reads everything through typed
wrappers in `team-frontend/src/lib/gateway/*.ts`, which call the gateway's Connect forwarders. The gateway
forwarders are pure pass-through (`edge/listing.go`).

## Goals / Non-Goals

**Goals:** one owner for the name; the cart, PDP header, `/shop/<id>`, `/account/following` and seller
pages can render it; additive proto; no N+1; safe fallback.
**Non-goals:** see proposal.

## Decisions

### D1. Owner = team-domain storefront (not team-identity)

| Option | Verdict |
|---|---|
| A. team-identity (`username` / new profile) | Rejected. `username` is the login handle (PII-ish, not meant to be public). A new profile would put shop presentation in the auth service and add a hop for a public read. |
| B. **team-domain `Storefront.display_name`** | **Chosen.** The shop page already lives here, is seller-owned, owner-enforced and public. Adding a field is the smallest change and needs no migration (it goes in the `config` JSONB). |
| C. New "shop" service | Rejected. A new bounded context for one string. |

### D2. Distribution = batch RPC read by the frontend wrapper, not copied data, not gateway composition

| Option | Trade-off | Verdict |
|---|---|---|
| 1. Add `seller_display_name` to team-order `CartItem` and team-engagement follow rows (owner pushes or caller copies) | Violates "one owner": the name is stale after a rename; needs events or a backfill; order/engagement need a domain client. | Rejected |
| 2. Gateway enriches `GetCart` / `ListFollowedSellers` responses | Needs new proto fields on order and engagement messages anyway, plus hand-written composition in three forwarders; this is orchestration drifting toward business logic (rule 2). | Rejected |
| 3. **`BatchGetStorefronts` RPC; the server-side wrapper joins** | One additive RPC, one owner, always fresh (no cache or copy). The join is UI shaping in `lib/gateway`, through the gateway only (rule 1). Cost: one extra gateway read per page that shows shops. | **Chosen** |
| 4. team-search read model (already denormalises seller data from Kafka) | Eventually consistent, only exists for listings in the index, and would need a new event plus a consumer for the cart and following pages. Over-built for this goal. | Rejected here; possible later for search facets |

### D3. Proto changes (additive only)

```proto
message Storefront { ...; string display_name = 7; }   // 1-80 chars after trim; empty = unset

rpc BatchGetStorefronts(BatchGetStorefrontsRequest) returns (BatchGetStorefrontsResponse);

message ShopSummary { string seller_id = 1; string display_name = 2; string slug = 3; }
message BatchGetStorefrontsRequest  { repeated string seller_ids = 1; }   // max 100, duplicates ignored
message BatchGetStorefrontsResponse { repeated ShopSummary shops = 1; }
```

A summary message (not `Storefront`) keeps the payload small (no featured ids or banner). Unknown sellers are
**omitted**; the caller treats a missing entry and an empty `display_name` identically. Gates:
`buf lint`, `buf breaking`, regenerate in team-domain, team-gateway and team-frontend (never hand-edited).

### D4. N+1 strategy

- Cart: collect `new Set(items.map(sellerId))` and make one `batchGetStorefronts` call after `getCart()`;
  attach `sellerDisplayName` to each item. A cart with 2 sellers costs 2 gateway reads total (cart + batch),
  not 1 + 2.
- Following: `listFollowedSellers()` (page size 48) followed by one batch call; the wrapper returns
  `{ sellerId, displayName }[]` (existing callers of the id-only signature are updated).
- `/shop/<id>` and PDP header: `getStorefront` already returns the full row, so the name is free; no batch.
- Server cap of 100 ids (above the page size 48 and any realistic cart). Wrapper chunks defensively at 100.
- Failure isolation: if the batch call fails, the wrapper logs, returns empty names and the page still
  renders with fallbacks. A name lookup never fails the cart or the following page.
- Within a single request the call is naturally de-duplicated by the `Set`; no cross-request cache (names
  must reflect renames immediately, see scenario).

### D5. Fallback rule

`shopLabel(sellerId, displayName)` = trimmed `displayName` if non-empty, else `Shop #` + first 6 chars of
`sellerId` (shorter ids are used whole; empty id gives `Shop`). It is pure, lives in one frontend module,
and is the only place the fallback string is built. Unknown seller, deleted storefront, a storefront with no
name, and a failed lookup all follow this path.

### D6. Validation and auth

`display_name` is trimmed, 1-80 runes when provided (control characters rejected); empty is allowed (means
"unset"). Write stays owner-scoped (`UpsertStorefront` forces `seller_id` to the principal). Reads
(`GetStorefront`, `BatchGetStorefronts`) expose only public shop data and follow the same auth posture as
`GetStorefront`. Values are rendered as text by React (no HTML), so no extra escaping work.

### D7. Events (rule 5)

None. Storefront writes emit no event today and the batch RPC is a synchronous read. No new topic, no
RabbitMQ.

## Risks / Trade-offs

- One extra round trip on cart and following pages. Mitigated by batching and failure isolation; the
  alternatives cost data duplication or staleness.
- Two sellers may choose the same display name (uniqueness is a non-goal; the slug is the unique handle).
- Existing shops show the fallback until sellers set a name; a seller edit form is the `ui-phase-seller`
  dependency.
- Frontend join: if the same join is later needed by many pages, promote to gateway composition or a
  read model via a follow-up ADR note.

## Migration Plan

1. Merge the proto PR (`buf lint`, `buf breaking` green). 2. Regenerate and deploy team-domain, then the
gateway, then the frontend (old clients are unaffected; the frontend wrapper treats an `Unimplemented`
batch error as "no names"). 3. No data migration; rollback = revert the frontend wrapper, the field and RPC
can stay.

## Open Questions

1. Should the name be required when a seller creates a storefront, or stay optional with the fallback?
   (Assumed optional; slug remains the only required field.)
2. Should a seller with no storefront row be auto-provisioned a default name at registration? That needs a
   team-identity to team-domain call or an event, so it is out of scope here.
3. Name length limit (80) and charset: confirm with product.
4. Should `/account/following` and chat headers share the batch helper now or in their own phases?
5. Should `team-search` denormalise the name later for seller facets (needs a `StorefrontChanged` event)?
