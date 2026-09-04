-- KYC submissions for seller/user verification. One row per submission; a user
-- may submit more than once (the latest by created_at is the current state).
-- Owned by team-verification. MOCK: doc_ref stores only an opaque reference /
-- storage key to a document — no real document bytes are ever stored here.
CREATE TABLE IF NOT EXISTS kyc_submissions (
    id          VARCHAR(64) PRIMARY KEY,
    user_id     VARCHAR(64) NOT NULL,
    doc_type    VARCHAR(64) NOT NULL,
    doc_ref     VARCHAR(512) NOT NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'PENDING'
                CHECK (status IN ('PENDING', 'VERIFIED', 'REJECTED')),
    reviewed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Status lookups fetch a user's most recent submission.
CREATE INDEX IF NOT EXISTS idx_kyc_submissions_user_id
    ON kyc_submissions (user_id, created_at DESC);
