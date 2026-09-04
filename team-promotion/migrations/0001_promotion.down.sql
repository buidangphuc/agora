-- 0001_promotion down — rollback flash-sale, reservation, and voucher tables.
DROP TABLE IF EXISTS flash_sale_campaigns;
DROP TABLE IF EXISTS voucher_reservations;
DROP TABLE IF EXISTS vouchers;
