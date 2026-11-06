DROP TABLE IF EXISTS referral_outbox_events;
DROP TABLE IF EXISTS referral_processed_events;
DROP TABLE IF EXISTS referral_buyer_orders;
DROP TABLE IF EXISTS referral_accounts;

DROP INDEX IF EXISTS idx_referral_rewards_user_released;
DROP INDEX IF EXISTS idx_referral_rewards_status_release;
DROP INDEX IF EXISTS uq_referral_rewards_order;
ALTER TABLE referral_rewards DROP CONSTRAINT IF EXISTS referral_rewards_status_check;
ALTER TABLE referral_rewards DROP COLUMN IF EXISTS referee_id;
ALTER TABLE referral_rewards DROP COLUMN IF EXISTS order_id;
ALTER TABLE referral_rewards DROP COLUMN IF EXISTS released_at;
ALTER TABLE referral_rewards DROP COLUMN IF EXISTS release_at;
ALTER TABLE referral_rewards DROP COLUMN IF EXISTS status;

ALTER TABLE referrals DROP CONSTRAINT IF EXISTS referrals_status_check;
ALTER TABLE referrals DROP COLUMN IF EXISTS attributed_at;
ALTER TABLE referrals DROP COLUMN IF EXISTS status;
