ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS term_days INT NOT NULL DEFAULT 30;
ALTER TABLE subscription_plans DROP CONSTRAINT IF EXISTS subscription_plans_term_days_positive;
ALTER TABLE subscription_plans ADD CONSTRAINT subscription_plans_term_days_positive CHECK (term_days > 0);

UPDATE subscription_plans SET price = 199000 WHERE id = 'plan_pro';
UPDATE subscription_plans SET price = 499000 WHERE id = 'plan_premium';

ALTER TABLE seller_subscriptions ADD COLUMN IF NOT EXISTS current_period_start TIMESTAMPTZ;
ALTER TABLE seller_subscriptions ADD COLUMN IF NOT EXISTS current_period_end TIMESTAMPTZ;
ALTER TABLE seller_subscriptions ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE seller_subscriptions ADD COLUMN IF NOT EXISTS pending_plan_id TEXT;
ALTER TABLE seller_subscriptions ADD COLUMN IF NOT EXISTS period_seq BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS seller_subscriptions_due_idx ON seller_subscriptions (current_period_end) WHERE status = 'active';

UPDATE seller_subscriptions s
SET current_period_start = clock_timestamp(),
    current_period_end = clock_timestamp() + (p.term_days * INTERVAL '1 day'),
    auto_renew = true
FROM subscription_plans p
WHERE p.id = s.plan_id AND s.status = 'active' AND p.price > 0 AND s.current_period_end IS NULL;
