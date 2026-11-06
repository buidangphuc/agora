import { Timestamp } from "@bufbuild/protobuf";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "@/lib/gateway/client";

import { getAccountAgeMs } from "./accountAge";

vi.mock("@/lib/gateway/client", () => ({ makeClients: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: vi.fn(() => ({ sub: "u1" })),
}));

const NOW = new Date("2026-11-06T12:00:00Z");
const DAY = 24 * 60 * 60 * 1000;

function stub(getMyAccount: ReturnType<typeof vi.fn>) {
  vi.mocked(makeClients).mockReturnValue({ auth: { getMyAccount } } as never);
  return getMyAccount;
}

function ago(ms: number): Timestamp {
  return Timestamp.fromDate(new Date(NOW.getTime() - ms));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("getAccountAgeMs", () => {
  it("W12 returns now minus created_at in milliseconds", async () => {
    stub(
      vi.fn().mockResolvedValue({
        userId: "u1",
        createdAt: ago(13 * DAY + 23 * 3600000),
      }),
    );
    await expect(getAccountAgeMs()).resolves.toBe(13 * DAY + 23 * 3600000);
  });

  it("W12 a 14 day old account reports exactly 14 days (boundary)", async () => {
    stub(vi.fn().mockResolvedValue({ userId: "u1", createdAt: ago(14 * DAY) }));
    await expect(getAccountAgeMs()).resolves.toBe(14 * DAY);
  });

  it("W20 a 30 day old account reports 30 days", async () => {
    stub(vi.fn().mockResolvedValue({ userId: "u1", createdAt: ago(30 * DAY) }));
    await expect(getAccountAgeMs()).resolves.toBe(30 * DAY);
  });

  it("W13 Age unknown hides the panel", async () => {
    stub(vi.fn().mockRejectedValue(new Error("unavailable")));
    await expect(getAccountAgeMs()).resolves.toBeNull();
  });

  it("W13 fails closed with null when created_at is missing", async () => {
    stub(vi.fn().mockResolvedValue({ userId: "u1" }));
    await expect(getAccountAgeMs()).resolves.toBeNull();
  });

  it("W13 fails closed with null when created_at is the zero timestamp", async () => {
    stub(
      vi.fn().mockResolvedValue({
        userId: "u1",
        createdAt: new Timestamp({ seconds: 0n }),
      }),
    );
    await expect(getAccountAgeMs()).resolves.toBeNull();
  });

  it("W21 bounds the read with AbortSignal.timeout(2000) passed to the rpc", async () => {
    const timeout = vi.spyOn(AbortSignal, "timeout");
    const rpc = stub(
      vi.fn().mockResolvedValue({ userId: "u1", createdAt: ago(DAY) }),
    );
    await getAccountAgeMs();
    expect(timeout).toHaveBeenCalledWith(2000);
    expect(rpc.mock.calls[0][1]).toEqual(
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it("W13 never exposes the user id in the result", async () => {
    stub(
      vi.fn().mockResolvedValue({ userId: "u-secret", createdAt: ago(DAY) }),
    );
    const res = await getAccountAgeMs();
    expect(JSON.stringify(res)).not.toContain("u-secret");
  });
});
