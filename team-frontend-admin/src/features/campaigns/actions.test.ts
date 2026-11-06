import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  type CampaignView,
  cancelCampaign,
  createCampaign,
  updateCampaign,
} from "@/lib/gateway/campaigns";
import { getPrincipal } from "@/lib/gateway/session";

import {
  cancelCampaignAction,
  createCampaignAction,
  updateCampaignAction,
} from "./actions";
import { campaignErrorMessage } from "./campaignMessages";

vi.mock("@/lib/gateway/campaigns", () => ({
  createCampaign: vi.fn(),
  cancelCampaign: vi.fn(),
  updateCampaign: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

const admin = {
  id: "u-admin",
  name: "Admin",
  scopes: ["admin", "promotion.admin"],
};
const seller = { id: "u-2", name: "Seller", scopes: ["listing.write"] };
const KEY = "retry-key-0001";

const draft = {
  listingId: "l-1",
  variantId: "",
  salePrice: "99000",
  stockCap: "50",
  startsAt: "2099-11-01T08:00",
  endsAt: "2099-11-01T12:00",
};

const campaign: CampaignView = {
  id: "fs-1",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 0,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
  status: "scheduled",
  cancelledAt: "",
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(createCampaign).mockResolvedValue({ status: "ok", campaign });
  vi.mocked(cancelCampaign).mockResolvedValue({
    status: "ok",
    campaign: { ...campaign, status: "cancelled" },
  });
  vi.mocked(updateCampaign).mockResolvedValue({ status: "ok", campaign });
});

describe("createCampaignAction", () => {
  it("creates with parsed input and the caller's idempotency key", async () => {
    const res = await createCampaignAction(draft, KEY);
    expect(createCampaign).toHaveBeenCalledWith(
      {
        listingId: "l-1",
        variantId: "",
        salePrice: 99000,
        stockCap: 50,
        startsAt: new Date("2099-11-01T01:00:00.000Z"),
        endsAt: new Date("2099-11-01T05:00:00.000Z"),
      },
      KEY,
    );
    expect(res).toEqual({ ok: true, campaign });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/campaigns");
  });

  it("redirects a guest and refuses a non-admin without calling the gateway", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(createCampaignAction(draft, KEY)).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/campaigns/new");
    vi.mocked(getPrincipal).mockReturnValue(seller);
    expect(await createCampaignAction(draft, KEY)).toMatchObject({
      ok: false,
    });
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("redirects a guest and refuses a non-admin without calling the gateway (admin token without promotion.admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(createCampaignAction(draft, KEY)).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/campaigns/new");
    vi.mocked(getPrincipal).mockReturnValue({ ...seller, scopes: ["admin"] });
    expect(await createCampaignAction(draft, KEY)).toMatchObject({
      ok: false,
    });
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("re-validates server-side and sends nothing for a bad window", async () => {
    const res = await createCampaignAction(
      { ...draft, endsAt: draft.startsAt },
      KEY,
    );
    expect(res).toMatchObject({
      ok: false,
      fieldErrors: { endsAt: expect.any(String) },
    });
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("server errors map to distinct messages and none leaks a raw backend message", async () => {
    const statuses = [
      "duplicate",
      "listing_unavailable",
      "audit_unavailable",
      "promotion_unavailable",
    ] as const;
    const seen: string[] = [];
    for (const status of statuses) {
      vi.mocked(createCampaign).mockResolvedValueOnce({ status });
      const res = await createCampaignAction(draft, KEY);
      expect(res).toMatchObject({ ok: false });
      if (!res.ok) {
        expect(res.error).toBe(campaignErrorMessage(status));
        seen.push(res.error);
      }
    }
    vi.mocked(createCampaign).mockResolvedValueOnce({ status: "invalid" });
    const invalid = await createCampaignAction(draft, KEY);
    if (!invalid.ok) seen.push(invalid.error);
    expect(invalid.ok).toBe(false);
    expect(new Set(seen).size).toBe(5);
  });

  it("flags a timeout as uncertain so the form keeps the key for a retry", async () => {
    vi.mocked(createCampaign).mockResolvedValueOnce({ status: "timeout" });
    expect(await createCampaignAction(draft, KEY)).toMatchObject({
      ok: false,
      uncertain: true,
    });
  });
});

describe("cancelCampaignAction", () => {
  it("cancels and revalidates the list", async () => {
    const res = await cancelCampaignAction("fs-1");
    expect(cancelCampaign).toHaveBeenCalledWith("fs-1");
    expect(res).toMatchObject({
      ok: true,
      campaign: { id: "fs-1", status: "cancelled" },
    });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/campaigns");
  });

  it("refuses a non-admin without calling the gateway and redirects a guest", async () => {
    vi.mocked(getPrincipal).mockReturnValue(seller);
    expect(await cancelCampaignAction("fs-1")).toMatchObject({ ok: false });
    expect(cancelCampaign).not.toHaveBeenCalled();
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(cancelCampaignAction("fs-1")).rejects.toThrow("NEXT_REDIRECT");
    expect(cancelCampaign).not.toHaveBeenCalled();
  });

  it("refuses a non-admin without calling the gateway and redirects a guest (admin token without promotion.admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...seller, scopes: ["admin"] });
    expect(await cancelCampaignAction("fs-1")).toMatchObject({ ok: false });
    expect(cancelCampaign).not.toHaveBeenCalled();
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(cancelCampaignAction("fs-1")).rejects.toThrow("NEXT_REDIRECT");
    expect(cancelCampaign).not.toHaveBeenCalled();
  });

  it("audit outage is reported with the audit message and the list is not revalidated", async () => {
    vi.mocked(cancelCampaign).mockResolvedValueOnce({
      status: "audit_unavailable",
    });
    const res = await cancelCampaignAction("fs-1");
    expect(res).toMatchObject({ ok: false });
    if (!res.ok) expect(res.error.length).toBeGreaterThan(0);
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("updateCampaignAction", () => {
  const edit = {
    salePrice: "90000",
    stockCap: "40",
    startsAt: "2099-12-01T08:00",
    endsAt: "2099-12-01T12:00",
  };

  it("updates with parsed values", async () => {
    const res = await updateCampaignAction("fs-1", edit);
    expect(updateCampaign).toHaveBeenCalledWith("fs-1", {
      salePrice: 90000,
      stockCap: 40,
      startsAt: new Date("2099-12-01T01:00:00.000Z"),
      endsAt: new Date("2099-12-01T05:00:00.000Z"),
    });
    expect(res).toMatchObject({ ok: true });
  });

  it("refuses a non-admin and invalid values without calling the gateway", async () => {
    vi.mocked(getPrincipal).mockReturnValue(seller);
    expect(await updateCampaignAction("fs-1", edit)).toMatchObject({
      ok: false,
    });
    vi.mocked(getPrincipal).mockReturnValue(admin);
    expect(
      await updateCampaignAction("fs-1", { ...edit, stockCap: "0" }),
    ).toMatchObject({
      ok: false,
      fieldErrors: { stockCap: expect.any(String) },
    });
    expect(updateCampaign).not.toHaveBeenCalled();
  });

  it("refuses a non-admin and invalid values without calling the gateway (admin token without promotion.admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...seller, scopes: ["admin"] });
    expect(await updateCampaignAction("fs-1", edit)).toMatchObject({
      ok: false,
    });
    vi.mocked(getPrincipal).mockReturnValue(admin);
    expect(
      await updateCampaignAction("fs-1", { ...edit, stockCap: "0" }),
    ).toMatchObject({
      ok: false,
      fieldErrors: { stockCap: expect.any(String) },
    });
    expect(updateCampaign).not.toHaveBeenCalled();
  });
});
