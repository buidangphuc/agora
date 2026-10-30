import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { createCampaign } from "@/lib/gateway/campaigns";
import { getPrincipal } from "@/lib/gateway/session";

import { createCampaignAction } from "./campaignActions";
import { CAMPAIGN_ERRORS } from "./campaignForm";

vi.mock("@/lib/gateway/campaigns", () => ({ createCampaign: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };

const draft = {
  listingId: "l-1",
  variantId: "",
  salePrice: "99000",
  stockCap: "50",
  startsAt: "2099-11-01T08:00",
  endsAt: "2099-11-01T12:00",
};

const campaign = {
  id: "fs-1",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 0,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(createCampaign).mockResolvedValue({ status: "ok", campaign });
});

describe("createCampaignAction", () => {
  it("creates with parsed input and returns the stored campaign", async () => {
    const res = await createCampaignAction(draft);
    expect(createCampaign).toHaveBeenCalledWith({
      listingId: "l-1",
      variantId: "",
      salePrice: 99000,
      stockCap: 50,
      startsAt: new Date("2099-11-01T01:00:00.000Z"),
      endsAt: new Date("2099-11-01T05:00:00.000Z"),
    });
    expect(res).toEqual({ ok: true, campaign });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/campaigns");
  });

  it("redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(createCampaignAction(draft)).rejects.toThrow("NEXT_REDIRECT");
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/campaigns/new");
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("refuses a non-admin without calling the gateway", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-2",
      name: "Seller",
      scopes: ["listing.write"],
    });
    expect(await createCampaignAction(draft)).toEqual({
      ok: false,
      error: CAMPAIGN_ERRORS.forbidden,
    });
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it("re-validates on the server and returns field errors", async () => {
    const res = await createCampaignAction({ ...draft, salePrice: "0" });
    expect(res).toMatchObject({
      ok: false,
      error: CAMPAIGN_ERRORS.invalid,
      fieldErrors: { salePrice: expect.any(String) },
    });
    expect(createCampaign).not.toHaveBeenCalled();
  });

  it.each([
    ["forbidden", CAMPAIGN_ERRORS.forbidden],
    ["invalid", CAMPAIGN_ERRORS.invalid],
    ["not_found", CAMPAIGN_ERRORS.notFound],
    ["unavailable", CAMPAIGN_ERRORS.unavailable],
  ] as const)("maps %s to its message", async (status, error) => {
    vi.mocked(createCampaign).mockResolvedValue({ status });
    expect(await createCampaignAction(draft)).toEqual({ ok: false, error });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("marks a timeout as uncertain and refreshes the list", async () => {
    vi.mocked(createCampaign).mockResolvedValue({ status: "timeout" });
    expect(await createCampaignAction(draft)).toEqual({
      ok: false,
      error: CAMPAIGN_ERRORS.timeout,
      uncertain: true,
    });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/campaigns");
  });

  it("sends a guest to login when the gateway rejects the session", async () => {
    vi.mocked(createCampaign).mockResolvedValue({ status: "unauthenticated" });
    await expect(createCampaignAction(draft)).rejects.toThrow("NEXT_REDIRECT");
  });
});
