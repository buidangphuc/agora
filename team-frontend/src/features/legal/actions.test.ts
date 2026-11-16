import { beforeEach, describe, expect, it, vi } from "vitest";

vi.hoisted(() => {
  process.env.NEXT_PUBLIC_LEGAL_TERMS_VERSION = "2026-11";
  process.env.NEXT_PUBLIC_PRIVACY_POLICY_VERSION = "2026-11-14";
});

const gw = vi.hoisted(() => ({
  listConsents: vi.fn(),
  recordConsents: vi.fn(),
  getToken: vi.fn(),
}));

vi.mock("@/lib/gateway/privacy", () => ({
  listConsents: gw.listConsents,
  recordConsents: gw.recordConsents,
}));
vi.mock("@/lib/gateway/session", () => ({
  getToken: gw.getToken,
  SESSION_COOKIE: "session",
}));

import { acceptCurrentLegal, getLegalAcceptance } from "./actions";

const T = "2026-11";
const P = "2026-11-14";

function row(purpose: string, version: string, granted = true) {
  return { purpose, version, granted, source: "register" };
}

beforeEach(() => {
  vi.clearAllMocks();
  gw.getToken.mockReturnValue("tok");
});

describe("getLegalAcceptance", () => {
  it("[Anonymous visitors are never prompted] no session returns signedIn:false with 0 gateway calls", async () => {
    gw.getToken.mockReturnValue(undefined);
    await expect(getLegalAcceptance()).resolves.toEqual({ signedIn: false });
    expect(gw.listConsents).not.toHaveBeenCalled();
    expect(gw.recordConsents).not.toHaveBeenCalled();
  });

  it("[A legacy user is prompted and accepts] no TERMS or PRIVACY_POLICY row needs acceptance", async () => {
    gw.listConsents.mockResolvedValue({
      records: [row("ANALYTICS", "1")],
      nextCursor: "",
    });
    await expect(getLegalAcceptance()).resolves.toEqual({
      signedIn: true,
      needsAcceptance: true,
    });
  });

  it("[Up-to-date users are not prompted] latest TERMS and PRIVACY_POLICY equal the current versions", async () => {
    gw.listConsents.mockResolvedValue({
      records: [row("TERMS", T), row("PRIVACY_POLICY", P)],
      nextCursor: "",
    });
    await expect(getLegalAcceptance()).resolves.toEqual({
      signedIn: true,
      needsAcceptance: false,
    });
    expect(gw.listConsents).toHaveBeenCalledTimes(1);
  });

  it("[A newer Terms version triggers the prompt] an older latest TERMS version needs acceptance", async () => {
    gw.listConsents.mockResolvedValue({
      records: [row("TERMS", "2026-01-01"), row("PRIVACY_POLICY", P)],
      nextCursor: "",
    });
    await expect(getLegalAcceptance()).resolves.toMatchObject({
      signedIn: true,
      needsAcceptance: true,
    });
  });

  it("[A newer Terms version triggers the prompt] an older latest PRIVACY_POLICY version needs acceptance", async () => {
    gw.listConsents.mockResolvedValue({
      records: [row("TERMS", T), row("PRIVACY_POLICY", "2026-01-01")],
      nextCursor: "",
    });
    await expect(getLegalAcceptance()).resolves.toMatchObject({
      needsAcceptance: true,
    });
  });

  it("[A newer Terms version triggers the prompt] the newest row per purpose wins (list is newest first)", async () => {
    gw.listConsents.mockResolvedValue({
      records: [
        row("TERMS", T),
        row("PRIVACY_POLICY", P),
        row("TERMS", "2025-01-01"),
      ],
      nextCursor: "",
    });
    await expect(getLegalAcceptance()).resolves.toMatchObject({
      needsAcceptance: false,
    });
  });

  it("[Up-to-date users are not prompted] pages with the returned cursor until both purposes are found", async () => {
    gw.listConsents
      .mockResolvedValueOnce({
        records: [row("TERMS", T)],
        nextCursor: "c2",
      })
      .mockResolvedValueOnce({
        records: [row("PRIVACY_POLICY", P)],
        nextCursor: "",
      });
    await expect(getLegalAcceptance()).resolves.toMatchObject({
      needsAcceptance: false,
    });
    expect(gw.listConsents).toHaveBeenCalledTimes(2);
    expect(gw.listConsents.mock.calls[1][0]).toBe("c2");
  });

  it("[A legacy user is prompted and accepts] reads at most 5 pages when the rows never appear", async () => {
    gw.listConsents.mockResolvedValue({
      records: [row("ANALYTICS", "1")],
      nextCursor: "more",
    });
    await expect(getLegalAcceptance()).resolves.toMatchObject({
      needsAcceptance: true,
    });
    expect(gw.listConsents).toHaveBeenCalledTimes(5);
  });

  it("[Consent lookup failure does not block browsing] a failed lookup shows no prompt and does not throw", async () => {
    gw.listConsents.mockRejectedValue(new Error("UNAVAILABLE"));
    await expect(getLegalAcceptance()).resolves.toMatchObject({
      signedIn: true,
      needsAcceptance: false,
    });
  });
});

describe("acceptCurrentLegal", () => {
  it("[A legacy user is prompted and accepts] records TERMS and PRIVACY_POLICY at the current versions once", async () => {
    gw.recordConsents.mockResolvedValue(undefined);
    await expect(acceptCurrentLegal()).resolves.toMatchObject({ ok: true });
    expect(gw.recordConsents).toHaveBeenCalledTimes(1);
    const recs = gw.recordConsents.mock.calls[0][0] as {
      purpose: string;
      granted: boolean;
      version: string;
      source: string;
    }[];
    expect(recs).toHaveLength(2);
    expect(recs).toContainEqual({
      purpose: "TERMS",
      granted: true,
      version: T,
      source: "account",
    });
    expect(recs).toContainEqual({
      purpose: "PRIVACY_POLICY",
      granted: true,
      version: P,
      source: "account",
    });
  });

  it("[Anonymous visitors are never prompted] without a session nothing is recorded", async () => {
    gw.getToken.mockReturnValue(undefined);
    await expect(acceptCurrentLegal()).resolves.toMatchObject({ ok: false });
    expect(gw.recordConsents).not.toHaveBeenCalled();
  });

  it("[A legacy user is prompted and accepts] a gateway failure resolves ok:false instead of throwing", async () => {
    gw.recordConsents.mockRejectedValue(new Error("UNAVAILABLE"));
    await expect(acceptCurrentLegal()).resolves.toMatchObject({ ok: false });
  });
});
