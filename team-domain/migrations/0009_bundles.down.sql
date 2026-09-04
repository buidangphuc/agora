-- Roll back 0009_bundles.
DROP INDEX IF EXISTS bundles_seller_id_idx;
DROP TABLE IF EXISTS bundles;
