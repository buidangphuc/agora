import { Code, ConnectError, createPromiseClient } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getDemandForecast, getRevenueBreakdown } from "./analytics.js";

vi.mock("server-only", () => ({}));
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("@connectrpc/connect", async (orig) => ({
  ...(await orig<typeof import("@connectrpc/connect")>()),
  createPromiseClient: vi.fn(),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

const rpc = {
  getDemandForecast: vi.fn(),
  getRevenueBreakdown: vi.fn(),
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(createPromiseClient).mockReturnValue(rpc as never);
});

describe("getDemandForecast", () => {
  it("Forecast request is fixed at fourteen days: requests a 14 day horizon for the principal", async () => {
    rpc.getDemandForecast.mockResolvedValue({
      sellerId: "me",
      listingId: "l1",
      dailyForecasts: [{ date: "2026-11-14", p10: 1, p50: 2.5, p90: 4 }],
      suggestedReorderPoint: 12.2,
      safetyStock: 3,
      modelVersion: "baseline-v1",
      isColdStart: false,
      generatedAt: { seconds: 1_794_508_200n, nanos: 0 },
    });
    const view = await getDemandForecast("me", "l1");
    expect(rpc.getDemandForecast).toHaveBeenCalledTimes(1);
    expect(rpc.getDemandForecast.mock.calls[0][0]).toMatchObject({
      sellerId: "me",
      listingId: "l1",
      horizonDays: 14,
    });
    expect(view.modelVersion).toBe("baseline-v1");
    expect(view.isColdStart).toBe(false);
    expect(view.suggestedReorderPoint).toBe(12.2);
    expect(view.dailyForecasts).toEqual([
      { date: "2026-11-14", p10: 1, p50: 2.5, p90: 4 },
    ]);
  });

  it("Wrapper surfaces failure instead of zeros: rejects on Unavailable", async () => {
    const err = new ConnectError("store down", Code.Unavailable);
    rpc.getDemandForecast.mockRejectedValue(err);
    await expect(getDemandForecast("me", "l1")).rejects.toBe(err);
  });

  it("Gateway rejects the forecast call without a token: rejects on Unauthenticated", async () => {
    const err = new ConnectError("no token", Code.Unauthenticated);
    rpc.getDemandForecast.mockRejectedValue(err);
    await expect(getDemandForecast("me", "l1")).rejects.toBe(err);
  });
});

describe("getRevenueBreakdown range", () => {
  it("Range tab selects the candidate listings: forwards from and to", async () => {
    rpc.getRevenueBreakdown.mockResolvedValue({ days: [], topSkus: [] });
    const to = new Date("2026-11-15T00:00:00Z");
    const from = new Date("2026-10-16T00:00:00Z");
    await getRevenueBreakdown("me", { throwOnError: true, from, to });
    const req = rpc.getRevenueBreakdown.mock.calls[0][0];
    expect(req.sellerId).toBe("me");
    expect(req.from?.toDate().toISOString()).toBe(from.toISOString());
    expect(req.to?.toDate().toISOString()).toBe(to.toISOString());
  });

  it("without a range sends no from or to", async () => {
    rpc.getRevenueBreakdown.mockResolvedValue({ days: [], topSkus: [] });
    await getRevenueBreakdown("me");
    const req = rpc.getRevenueBreakdown.mock.calls[0][0];
    expect(req.from).toBeUndefined();
    expect(req.to).toBeUndefined();
  });
});
