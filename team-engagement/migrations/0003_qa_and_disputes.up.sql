-- 0003_qa_and_disputes.up.sql — Schema for Product Q&A and Disputes.

CREATE TABLE IF NOT EXISTS product_questions (
    id            VARCHAR(64) PRIMARY KEY,
    listing_id    VARCHAR(64) NOT NULL,
    user_id       VARCHAR(64) NOT NULL,
    question_text TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_product_questions_listing_id ON product_questions (listing_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_product_questions_user_id ON product_questions (user_id);

CREATE TABLE IF NOT EXISTS product_answers (
    id            VARCHAR(64) PRIMARY KEY,
    question_id   VARCHAR(64) NOT NULL REFERENCES product_questions(id) ON DELETE CASCADE,
    listing_id    VARCHAR(64) NOT NULL DEFAULT '',
    user_id       VARCHAR(64) NOT NULL,
    answer_text   TEXT NOT NULL,
    is_shop_reply BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_product_answers_question_id ON product_answers (question_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_product_answers_user_id ON product_answers (user_id);

CREATE TABLE IF NOT EXISTS disputes (
    id            VARCHAR(64) PRIMARY KEY,
    order_id      VARCHAR(64) NOT NULL,
    claimant_id   VARCHAR(64) NOT NULL,
    defendant_id  VARCHAR(64) NOT NULL,
    reason        TEXT NOT NULL,
    evidence_urls TEXT[] NOT NULL DEFAULT '{}',
    status        VARCHAR(32) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'INVESTIGATING', 'RESOLVED', 'REJECTED')),
    resolution    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_disputes_order_id ON disputes (order_id);
CREATE INDEX IF NOT EXISTS idx_disputes_claimant_id ON disputes (claimant_id);
CREATE INDEX IF NOT EXISTS idx_disputes_defendant_id ON disputes (defendant_id);
CREATE INDEX IF NOT EXISTS idx_disputes_status ON disputes (status);
