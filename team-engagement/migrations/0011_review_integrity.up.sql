-- 0011_review_integrity.up.sql — review integrity (engagement-review-integrity).
-- Moderation columns, and at most one verified review per (order, listing).

ALTER TABLE reviews ADD COLUMN IF NOT EXISTS hidden_at   TIMESTAMPTZ;
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS hidden_by   VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS hide_reason TEXT        NOT NULL DEFAULT '';

-- Demote (never delete) later duplicates: keep the earliest created_at, id as tiebreak.
UPDATE reviews r SET verified_purchase = FALSE
WHERE r.verified_purchase
  AND EXISTS (
    SELECT 1 FROM reviews k
    WHERE k.verified_purchase
      AND k.order_id = r.order_id
      AND k.listing_id = r.listing_id
      AND (k.created_at, k.id) < (r.created_at, r.id)
  );

CREATE UNIQUE INDEX IF NOT EXISTS uq_reviews_order_listing_verified
    ON reviews (order_id, listing_id) WHERE verified_purchase;
