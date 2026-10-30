import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal, hasSellerAccess } from "@/lib/gateway/session";

vi.mock("@/features/auth/LoginForm", () => ({
  LoginForm: ({ returnUrl }: { returnUrl?: string }) => (
    <div data-testid="form" data-return-url={returnUrl ?? ""} />
  ),
}));

vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(() => null),
  hasSellerAccess: vi.fn(() => false),
}));

import Page from "./page";

beforeEach(() => {
  vi.mocked(redirect).mockClear();
  vi.mocked(getPrincipal).mockReturnValue(null);
  vi.mocked(hasSellerAccess).mockReturnValue(false);
});

async function renderPage(sp: Record<string, string | string[] | undefined>) {
  render(await Page({ searchParams: Promise.resolve(sp) }));
  return screen.getByTestId("form").getAttribute("data-return-url");
}

describe("login page returnUrl (frontend-login-return)", () => {
  it("passes a safe searchParams.returnUrl to the form", async () => {
    expect(await renderPage({ returnUrl: "/listing/abc" })).toBe(
      "/listing/abc",
    );
  });

  it("passes no returnUrl when absent", async () => {
    expect(await renderPage({})).toBe("");
  });

  it("does not pass an unsafe returnUrl to the form", async () => {
    const v = await renderPage({ returnUrl: "https://evil.example" });
    expect(v).not.toContain("evil");
  });
});

describe("login page next param (cross-app entry from the buyer app)", () => {
  const seller = { id: "s1", name: "shop", scopes: ["listing.write"] };

  it("passes a safe next to the form", async () => {
    expect(await renderPage({ next: "/seller/orders" })).toBe("/seller/orders");
  });

  it("prefers next over returnUrl", async () => {
    expect(
      await renderPage({ next: "/seller", returnUrl: "/seller/new" }),
    ).toBe("/seller");
  });

  it("drops an unsafe next", async () => {
    expect(await renderPage({ next: "//evil.example" })).toBe("");
  });

  it("sends a signed-in seller straight to next", async () => {
    vi.mocked(getPrincipal).mockReturnValue(seller);
    vi.mocked(hasSellerAccess).mockReturnValue(true);
    await Page({ searchParams: Promise.resolve({ next: "/seller/orders" }) });
    expect(redirect).toHaveBeenCalledWith("/seller/orders");
  });

  it("sends a signed-in seller without next to the seller centre", async () => {
    vi.mocked(getPrincipal).mockReturnValue(seller);
    vi.mocked(hasSellerAccess).mockReturnValue(true);
    await Page({ searchParams: Promise.resolve({}) });
    expect(redirect).toHaveBeenCalledWith("/seller");
  });

  it("shows the form to a signed-in user without seller access", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...seller, scopes: [] });
    expect(await renderPage({ next: "/seller" })).toBe("/seller");
    expect(redirect).not.toHaveBeenCalled();
  });
});
