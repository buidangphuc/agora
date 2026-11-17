## Context

team-promotion already has `ad_campaigns` (migration 0003), `AdCampaignRepository` and `sponsored.Service`. The buyer
frontend renders slots server side and must degrade to organic results; the gateway only forwards.

## Decisions

- **Placements.** `placement` is `search` or `home`; anything else is `INVALID_ARGUMENT`. `search` requires a non-empty
  `category_id` and matches the campaign's stored `category_id`; with an empty category it returns no slots (relevance in
  v1 is category match). `home` ignores `category_id`. The "page 1 only" rule for search is enforced by the caller (the
  frontend only asks on page 1); the service has no page notion. Empty `placement` is the legacy call: up to 50
  `listing_ids` by bid, no slots, no tokens, no impression count.
- **Ranking and dedupe.** `ORDER BY bid DESC, created_at ASC`, then in memory one entry per `listing_id` (the first), cut to
  `limit`. Eligibility: `status = ACTIVE AND budget - spent >= bid`.
- **Category source.** Stored at create time from the `GetListing` response already fetched for the ownership check; no
  extra RPC at serve time. Later category edits of a listing are not propagated (accepted v1). Admin-created campaigns skip
  the listing lookup (existing audit spec) and store an empty category, so they appear on `home` only.
- **Impression token.** 16 random bytes, base64url, primary key of `ad_impressions(token, campaign_id, listing_id,
  created_at, expires_at, clicked_at)`. Slots of one response are inserted in one batched statement together with
  `impressions = impressions + 1` on their campaigns.
- **Click charge.** One transaction: (1) `UPDATE ad_impressions SET clicked_at = now() WHERE token = $1 AND clicked_at IS
  NULL AND expires_at > now() RETURNING campaign_id`; if no row, look the token up read-only to return `listing_id` with
  `charged = false` (unknown token is `NOT_FOUND`); (2) self-click check against the forwarded principal: if equal, roll
  back so the token stays unconsumed; (3) `UPDATE ad_campaigns SET spent = spent + bid, clicks = clicks + 1, status = CASE
  WHEN budget - spent - bid < bid THEN PAUSED_BUDGET ELSE status END WHERE id = $2 AND status = ACTIVE AND budget - spent
  >= bid`; zero rows means the campaign cannot pay: roll back, `charged = false`. Row lock on the campaign serialises
  concurrent clicks, which is the compare-and-set.
- **First price.** The charge equals `ad_campaigns.bid` at click time. v1 has no bid edit, so it equals the bid at serve time.
- **Min bid and budget.** `MIN_AD_BID` config, default 500, validated >= 1 at startup, next to `MAX_AD_BID`. A budget below
  the bid is rejected because the campaign could never be served.
- **Migration.** `ALTER TABLE ad_campaigns ADD spent/impressions/clicks BIGINT NOT NULL DEFAULT 0, ADD category_id TEXT NOT
  NULL DEFAULT ''`; rows with `bid < 500` or `budget < bid` are set to status ENDED (3) because they could never serve;
  index on `(status, category_id, bid DESC, created_at)`. New table `ad_impressions` with an index on `created_at` for the
  purge and on `campaign_id`. Down migration drops the new objects. The file takes the next free number after
  promotion-plan-billing (hot-file serialization).
- **Purge.** The existing sweeper `Run` loop also deletes `ad_impressions` older than 30 days in batches of 100.
- **Gateway.** `SponsoredForwarder.RecordAdClick` mirrors `ListSponsoredSlots`: no scope, anonymous principal passes
  through. Self-click detection works because the edge forwards the resolved principal when a bearer token is present.

## Risks

- Heavy click fraud is not modelled; budgets are counters and rate limiting is the only brake.
- Impressions are counted when served, not when rendered, so a page that drops slots still counts.
