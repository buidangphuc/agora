ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders DROP COLUMN IF EXISTS paid_at;
DROP INDEX IF EXISTS order_sagas_buyer_idem_key_uidx;
ALTER TABLE order_sagas DROP COLUMN IF EXISTS idempotency_key;
