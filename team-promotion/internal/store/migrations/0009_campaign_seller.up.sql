ALTER TABLE flash_sale_campaigns ADD COLUMN IF NOT EXISTS seller_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS flash_sale_campaigns_seller_idx ON flash_sale_campaigns (seller_id) WHERE seller_id <> '';
