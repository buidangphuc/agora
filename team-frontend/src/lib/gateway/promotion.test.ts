import { Timestamp } from "@bufbuild/protobuf";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  DiscountType,
  VoucherKind,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

const rpc = vi.hoisted(() => ({
  listMyVouchers: vi.fn(),
  listVouchers: vi.fn(),
  listSponsoredSlots: vi.fn(),
}));
const session = vi.hoisted(() => ({
  getPrincipal: vi.fn(),
}));

vi.mock("@connectrpc/connect", async (importOriginal) => {
  const orig = await importOriginal<typeof import("@connectrpc/connect")>();
  return { ...orig, createPromiseClient: () => rpc };
});
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: () => ({}),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: session.getPrincipal,
  SESSION_COOKIE: "session",
}));

import {
  listMyVouchers,
  listSponsoredSlotSet,
  listVouchers,
} from "./promotion.js";

function protoVoucher(over: Record<string, unknown> = {}) {
  return {
    id: "v1",
    code: "WELCOME50",
    scope: VoucherScope.PLATFORM,
    sellerId: "",
    discountType: DiscountType.FIXED,
    discountValue: 50000n,
    minSpend: 200000n,
    maxDiscount: 0n,
    quota: 1n,
    used: 0n,
    startsAt: new Timestamp({ seconds: 1793000000n }),
    endsAt: new Timestamp({ seconds: 1795118400n }),
    kind: VoucherKind.NEW_USER,
    ownerUserId: "u1",
    ...over,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  session.getPrincipal.mockReturnValue({ sub: "u1" });
});

afterEach(() => vi.restoreAllMocks());

describe("listMyVouchers", () => {
  it("W1 returns an ok result with the vouchers mapped including kind and ownerUserId", async () => {
    rpc.listMyVouchers.mockResolvedValue({ vouchers: [protoVoucher()] });
    const res = await listMyVouchers();
    expect(res.ok).toBe(true);
    if (!res.ok) return;
    expect(res.vouchers).toHaveLength(1);
    expect(res.vouchers[0]).toMatchObject({
      code: "WELCOME50",
      discountValue: 50000,
      minSpend: 200000,
      quota: 1,
      used: 0,
      kind: VoucherKind.NEW_USER,
      ownerUserId: "u1",
      endsAt: "20/11/2026",
    });
  });

  it("W2 an empty list is an ok result with no vouchers (boundary)", async () => {
    rpc.listMyVouchers.mockResolvedValue({ vouchers: [] });
    await expect(listMyVouchers()).resolves.toEqual({
      ok: true,
      vouchers: [],
    });
  });

  it("W3 a failed read is a failed result, not an empty ok result", async () => {
    rpc.listMyVouchers.mockRejectedValue(new Error("unavailable"));
    await expect(listMyVouchers()).resolves.toEqual({ ok: false });
  });

  it("W4 an anonymous visitor short-circuits to an empty ok result without any rpc", async () => {
    session.getPrincipal.mockReturnValue(null);
    await expect(listMyVouchers()).resolves.toEqual({
      ok: true,
      vouchers: [],
    });
    expect(rpc.listMyVouchers).not.toHaveBeenCalled();
  });

  it("W5 sends no user id in the request (the principal comes from the bearer)", async () => {
    rpc.listMyVouchers.mockResolvedValue({ vouchers: [] });
    await listMyVouchers();
    expect(rpc.listMyVouchers).toHaveBeenCalledTimes(1);
    const req = rpc.listMyVouchers.mock.calls[0][0] ?? {};
    expect(Object.keys(req)).not.toContain("userId");
    expect(Object.keys(req)).not.toContain("ownerUserId");
  });

  it("W27 Picker read times out", async () => {
    const timeout = vi.spyOn(AbortSignal, "timeout");
    rpc.listMyVouchers.mockResolvedValue({ vouchers: [] });
    await listMyVouchers();
    expect(timeout).toHaveBeenCalledWith(2000);
    expect(rpc.listMyVouchers.mock.calls[0][1]).toEqual(
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it("W7 a read aborted by the timeout is a failed result", async () => {
    rpc.listMyVouchers.mockImplementation(
      (_req: unknown, opts?: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          opts?.signal?.addEventListener("abort", () =>
            reject(new Error("aborted")),
          );
          if (opts?.signal?.aborted) reject(new Error("aborted"));
        }),
    );
    vi.spyOn(AbortSignal, "timeout").mockImplementation(() => {
      const c = new AbortController();
      c.abort();
      return c.signal;
    });
    await expect(listMyVouchers()).resolves.toEqual({ ok: false });
  });
});

describe("listVouchers mapping", () => {
  it("W1 maps kind and ownerUserId on platform vouchers too", async () => {
    rpc.listVouchers.mockResolvedValue({
      vouchers: [protoVoucher({ kind: VoucherKind.STANDARD, ownerUserId: "" })],
    });
    const list = await listVouchers();
    expect(list[0]).toMatchObject({
      kind: VoucherKind.STANDARD,
      ownerUserId: "",
    });
  });
});

describe("listSponsoredSlotSet", () => {
  it("listSponsoredSlotSet maps slots", async () => {
    rpc.listSponsoredSlots.mockResolvedValue({
      listingIds: ["l1", "l2"],
      slots: [
        { listingId: "l1", campaignId: "c1", impressionToken: "tok-1" },
        { listingId: "l2", campaignId: "c2", impressionToken: "tok-2" },
      ],
    });

    const slots = await listSponsoredSlotSet({
      placement: "search",
      categoryId: "cat-9",
      limit: 2,
    });

    expect(slots).toEqual([
      { listingId: "l1", campaignId: "c1", impressionToken: "tok-1" },
      { listingId: "l2", campaignId: "c2", impressionToken: "tok-2" },
    ]);
    expect(rpc.listSponsoredSlots).toHaveBeenCalledTimes(1);
    expect(rpc.listSponsoredSlots.mock.calls[0][0]).toMatchObject({
      placement: "search",
      categoryId: "cat-9",
      limit: 2,
    });
  });

  it("listSponsoredSlotSet returns empty on error", async () => {
    rpc.listSponsoredSlots.mockRejectedValue(new Error("unavailable"));

    await expect(
      listSponsoredSlotSet({ placement: "home", categoryId: "", limit: 2 }),
    ).resolves.toEqual([]);
  });
});
