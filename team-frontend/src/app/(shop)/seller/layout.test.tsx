import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames } from "@/lib/gateway/shops";
import SellerLayout from "./layout";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/shops", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/gateway/shops")>()),
  batchGetShopNames: vi.fn(),
}));

const seller = {
  id: "zxcvbn123456",
  name: "Seller",
  scopes: ["listing.write"],
};

async function renderLayout() {
  render(await SellerLayout({ children: <p>page body</p> }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(seller);
  vi.mocked(batchGetShopNames).mockResolvedValue(new Map());
});

describe("SellerLayout", () => {
  it("redirects an unauthenticated visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    expect(await SellerLayout({ children: null })).toBeNull();
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("shows a 403 Result with a recovery link when listing.write is missing", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...seller, scopes: [] });
    await renderLayout();
    expect(screen.getByText("Cần tài khoản Người Bán")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại trang chủ" }),
    ).toHaveAttribute("href", "/");
    expect(screen.queryByText("page body")).toBeNull();
  });

  it("shows the real shop name on the shop card", async () => {
    vi.mocked(batchGetShopNames).mockResolvedValue(
      new Map([[seller.id, "Cửa hàng Hoa Mai"]]),
    );
    await renderLayout();
    expect(screen.getByText("page body")).toBeInTheDocument();
    expect(screen.getAllByText("Cửa hàng Hoa Mai").length).toBeGreaterThan(0);
    expect(screen.queryByText(/Shop #/)).toBeNull();
  });

  it("falls back to Shop # + 6 chars of the seller id for an empty name", async () => {
    await renderLayout();
    expect(screen.getAllByText("Shop #zxcvbn").length).toBeGreaterThan(0);
  });
});
