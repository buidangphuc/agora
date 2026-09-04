-- 0002_subscriptions down — rollback seller subscriptions and the plan catalog.
DROP TABLE IF EXISTS seller_subscriptions;
DROP TABLE IF EXISTS subscription_plans;
