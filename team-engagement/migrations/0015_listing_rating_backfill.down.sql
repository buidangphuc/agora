DELETE FROM engagement_outbox_events e
WHERE e.status = 'pending'
  AND e.event_type = 'platform.engagement.v1.ListingRatingChanged'
  AND e.event_id = md5('listing-rating-backfill:' || e.aggregate_id)::uuid::text;
