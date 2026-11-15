DROP TABLE IF EXISTS kyc_documents;
DROP TABLE IF EXISTS kyc_upload_grants;
ALTER TABLE kyc_submissions DROP COLUMN IF EXISTS superseded_at;
ALTER TABLE kyc_submissions DROP COLUMN IF EXISTS reason_code;
