-- 0005_voucher_redemption — record the promotion voucher applied at checkout and
-- the discount it granted on the order (Wave 1, W1-T2). Redemption itself is a
-- synchronous gRPC hold against team-promotion's VoucherService (ValidateAndReserve
-- during the saga, CommitReservation on PaymentSettled, ReleaseReservation on
-- compensation/cancel); these columns persist the applied result so total_amount is
-- auditable and the settle-time commit can address the hold by order id.

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS voucher_code    TEXT   NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS discount_amount BIGINT NOT NULL DEFAULT 0;
