import { beforeEach, describe, expect, it, vi } from "vitest";

const gw = vi.hoisted(() => ({
  recordConsents: vi.fn(),
  getToken: vi.fn(),
}));

vi.mock("@/lib/gateway/privacy", () => ({
  recordConsents: gw.recordConsents,
  listConsents: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getToken: gw.getToken,
  SESSION_COOKIE: "session",
}));

import { recordBannerConsent } from "./actions";

const prior = (a: boolean, m: boolean, p: boolean) => ({
  analytics: a,
  marketing: m,
  personalization: p,
  version: 1 as const,
});
const none = {
  analytics: false,
  marketing: false,
  personalization: false,
  version: 0 as const,
};

type Rec = {
  purpose: string;
  granted: boolean;
  version: string;
  source: string;
};

beforeEach(() => {
  vi.clearAllMocks();
  gw.getToken.mockReturnValue("tok");
  gw.recordConsents.mockResolvedValue(undefined);
});

describe("recordBannerConsent", () => {
  it("[Signed-in save records only changed purposes] one record for the single changed purpose", async () => {
    await recordBannerConsent(prior(true, false, false), {
      analytics: true,
      marketing: false,
      personalization: true,
    });
    expect(gw.recordConsents).toHaveBeenCalledTimes(1);
    expect(gw.recordConsents.mock.calls[0][0]).toEqual([
      {
        purpose: "PERSONALIZATION",
        granted: true,
        version: "1",
        source: "banner",
      },
    ]);
  });

  it("[Signed-in save records only changed purposes] a purpose switched off is recorded as granted=false", async () => {
    await recordBannerConsent(prior(true, true, false), {
      analytics: false,
      marketing: true,
      personalization: false,
    });
    expect(gw.recordConsents.mock.calls[0][0]).toEqual([
      { purpose: "ANALYTICS", granted: false, version: "1", source: "banner" },
    ]);
  });

  it("[First choice records all three purposes] no prior cookie records three denied rows in one call", async () => {
    await recordBannerConsent(none, {
      analytics: false,
      marketing: false,
      personalization: false,
    });
    expect(gw.recordConsents).toHaveBeenCalledTimes(1);
    const recs = gw.recordConsents.mock.calls[0][0] as Rec[];
    expect(recs).toHaveLength(3);
    expect(recs.map((r) => r.purpose).sort()).toEqual([
      "ANALYTICS",
      "MARKETING",
      "PERSONALIZATION",
    ]);
    for (const r of recs) {
      expect(r).toMatchObject({
        granted: false,
        version: "1",
        source: "banner",
      });
    }
  });

  it("[First choice records all three purposes] no prior cookie and accept-all records three granted rows", async () => {
    await recordBannerConsent(none, {
      analytics: true,
      marketing: true,
      personalization: true,
    });
    const recs = gw.recordConsents.mock.calls[0][0] as Rec[];
    expect(recs).toHaveLength(3);
    expect(recs.every((r) => r.granted)).toBe(true);
  });

  it("[Anonymous choice is cookie only] no session means 0 gateway calls", async () => {
    gw.getToken.mockReturnValue(undefined);
    await recordBannerConsent(none, {
      analytics: true,
      marketing: true,
      personalization: true,
    });
    expect(gw.recordConsents).not.toHaveBeenCalled();
  });

  it("[Ledger write failure keeps the cookie choice] a gateway failure is swallowed, not thrown", async () => {
    gw.recordConsents.mockRejectedValue(new Error("UNAVAILABLE"));
    await expect(
      recordBannerConsent(none, {
        analytics: true,
        marketing: false,
        personalization: false,
      }),
    ).resolves.not.toThrow();
  });
});
