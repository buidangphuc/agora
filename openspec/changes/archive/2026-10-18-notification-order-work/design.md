# Design

## Clarifications

### Round 1 — 2026-10-16

- B10 [notification-scope] Order work: An expiry cancel notifies the buyer; Order work: A buyer's own cancel creates no notification; Order work: A completed order prompts the buyer to review; Order work: A refunded return notifies the buyer: Which order events notify the buyer? → **ReturnRefunded yes; OrderCancelled only when reason_code is PAYMENT_EXPIRED; OrderCompleted always (any completed_by)** (human)
