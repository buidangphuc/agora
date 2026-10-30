import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal, getToken } from "@/lib/gateway/session";

import NewCampaignPage, { metadata } from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/features/admin/campaignActions", () => ({
  createCampaignAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "u-admin",
    name: "Admin",
    scopes: ["admin"],
  });
  vi.mocked(getToken).mockReturnValue("tok");
});

describe("NewCampaignPage", () => {
  it("declares no-referrer metadata", () => {
    expect(metadata.referrer).toBe("no-referrer");
  });

  it("redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await NewCampaignPage()).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/campaigns/new")}`,
    );
  });

  it("shows 403 to a non-admin", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-2",
      name: "Buyer",
      scopes: ["order.read"],
    });
    render(await NewCampaignPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(screen.queryByTestId("campaign-form")).toBeNull();
  });

  it("renders the form for an admin", async () => {
    render(await NewCampaignPage());
    expect(
      screen.getByRole("heading", { name: "Tạo flash sale" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("campaign-form")).toBeInTheDocument();
  });
});
