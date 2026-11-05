DROP TABLE IF EXISTS campaign_idempotency;
ALTER TABLE flash_sale_campaigns DROP COLUMN IF EXISTS cancelled_at;
