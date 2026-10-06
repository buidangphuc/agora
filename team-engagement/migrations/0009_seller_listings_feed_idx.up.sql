-- 0009: follow feed is newest-first; index the sort key and the by-listing
-- lookup used when a listing is deleted/unpublished or changes owner.
CREATE INDEX IF NOT EXISTS seller_listings_recent_idx ON seller_listings (seller_id, created_at DESC, listing_id DESC);
CREATE INDEX IF NOT EXISTS seller_listings_listing_idx ON seller_listings (listing_id);
