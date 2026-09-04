-- team-referral schema. Owned exclusively by team-referral; no other service
-- writes these tables. Rewards are a mock coin-credit ledger — accounting intent
-- only, no real money movement (see 00-spec / AGENTS.md §7).

-- One referral code per user. user_id is the natural PK; code is globally unique
-- so it can be resolved back to its owner on redeem.
CREATE TABLE IF NOT EXISTS referral_codes (
    user_id    VARCHAR(64) PRIMARY KEY,
    code       VARCHAR(32) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Each referee is attributed to exactly one referrer (unique referee_id), so a
-- user can be referred at most once. referrer_id + code record who invited them.
CREATE TABLE IF NOT EXISTS referrals (
    referrer_id VARCHAR(64) NOT NULL,
    referee_id  VARCHAR(64) NOT NULL UNIQUE,
    code        VARCHAR(32) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Referrers are looked up by their id to count invites and to append rewards.
CREATE INDEX IF NOT EXISTS idx_referrals_referrer ON referrals (referrer_id);

-- Append-only mock coin-credit ledger. One row per reward-earning event
-- (e.g. an invitee joining). amount is minor units.
CREATE TABLE IF NOT EXISTS referral_rewards (
    id         VARCHAR(64) PRIMARY KEY,
    user_id    VARCHAR(64) NOT NULL,
    amount     BIGINT NOT NULL,
    reason     VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Rewards are listed per user, newest-first.
CREATE INDEX IF NOT EXISTS idx_referral_rewards_user ON referral_rewards (user_id, created_at DESC);
