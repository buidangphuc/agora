DROP INDEX IF EXISTS flash_sale_campaigns_seller_idx;
ALTER TABLE flash_sale_campaigns DROP COLUMN IF EXISTS seller_id;
