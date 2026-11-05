import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames } from "@/lib/gateway/shops";
import SellerLayout from "./layout";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("next/headers", () => ({
  headers: vi.fn(async () => new Headers()),
}));
vi.mock("@/features/auth/actions", () => ({ logoutAction: vi.fn() }));
vi.mock("@/features/handoff/BuyerStorefrontLink", () => ({
  BuyerStorefrontLink: ({ children }: { children?: unknown }) => (
    <span data-testid="buyer-handoff-link">{children as never}</span>
  ),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasSellerAccess: () => true,
}));
vi.mock("@/lib/gateway/shops", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/gateway/shops")>()),
  batchGetShopNames: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "zxcvbn123456",
    name: "Seller",
    scopes: ["listing.write"],
  } as never);
  vi.mocked(batchGetShopNames).mockResolvedValue(new Map());
});

describe("seller layout buyer storefront link", () => {
  it("[Seller to buyer signed in] the layout offers the buyer storefront through BuyerStorefrontLink", async () => {
    render(await SellerLayout({ children: <p>page body</p> }));
    expect(screen.getAllByTestId("buyer-handoff-link").length).toBeGreaterThan(
      0,
    );
  });
});
