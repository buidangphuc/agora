ALTER TABLE kyc_submissions ADD COLUMN IF NOT EXISTS reason_code VARCHAR(64);
ALTER TABLE kyc_submissions ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS kyc_upload_grants (
    object_key   VARCHAR(512) PRIMARY KEY,
    user_id      VARCHAR(64) NOT NULL,
    content_type VARCHAR(64) NOT NULL,
    size_bytes   BIGINT NOT NULL,
    issued_at    TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_kyc_upload_grants_user_open
    ON kyc_upload_grants (user_id, expires_at) WHERE used_at IS NULL;

CREATE TABLE IF NOT EXISTS kyc_documents (
    submission_id VARCHAR(64) NOT NULL REFERENCES kyc_submissions (id) ON DELETE CASCADE,
    doc_index     INT NOT NULL,
    object_key    VARCHAR(512) NOT NULL UNIQUE,
    content_type  VARCHAR(64) NOT NULL,
    size_bytes    BIGINT NOT NULL,
    deleted_at    TIMESTAMPTZ,
    PRIMARY KEY (submission_id, doc_index)
);
