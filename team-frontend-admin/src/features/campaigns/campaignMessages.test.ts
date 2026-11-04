import { describe, expect, it } from "vitest";

import { campaignErrorMessage } from "./campaignMessages";

const STATUSES = [
  "duplicate",
  "invalid",
  "listing_unavailable",
  "audit_unavailable",
  "promotion_unavailable",
] as const;

const RAW_TOKENS = [
  "ALREADY_EXISTS",
  "INVALID_ARGUMENT",
  "LISTING_UNAVAILABLE",
  "AUDIT_UNAVAILABLE",
  "PROMOTION_UNAVAILABLE",
  "promotion.platform",
];

describe("campaign error messages", () => {
  it("maps each error: five statuses give five different messages", () => {
    const messages = STATUSES.map((s) => campaignErrorMessage(s));
    for (const m of messages) expect(m.trim().length).toBeGreaterThan(0);
    expect(new Set(messages).size).toBe(5);
  });

  it("maps each error: no message exposes a raw backend code or domain", () => {
    for (const s of STATUSES) {
      const m = campaignErrorMessage(s);
      for (const token of RAW_TOKENS) expect(m).not.toContain(token);
    }
  });
});
