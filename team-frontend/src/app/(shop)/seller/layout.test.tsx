import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";
import { batchGetShopNames } from "@/lib/gateway/shops";
import SellerLayout from "./layout";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller",
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  notFound: vi.fn(),
}));
const headerValues = vi.hoisted(() => ({ pathname: null as string | null }));
vi.mock("next/headers", () => ({
  headers: vi.fn(async () => {
    const h = new Headers();
    if (headerValues.pathname !== null)
      h.set("x-pathname", headerValues.pathname);
    return h;
  }),
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
  headerValues.pathname = null;
  vi.mocked(getPrincipal).mockReturnValue(seller);
  vi.mocked(batchGetShopNames).mockResolvedValue(new Map());
});

describe("SellerLayout", () => {
  it("[Param-less layout uses the x-pathname header] uses x-pathname header", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    headerValues.pathname = "/seller/orders";
    await expect(SellerLayout({ children: null })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fseller%2Forders",
    );
  });

  it("[Param-less layout uses the x-pathname header] keeps the query carried by x-pathname", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    headerValues.pathname = "/seller/orders?status=paid";
    await expect(SellerLayout({ children: null })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fseller%2Forders%3Fstatus%3Dpaid",
    );
  });

  it("[Param-less layout uses the x-pathname header] absent header redirects to plain /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(SellerLayout({ children: null })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("[Over-length path falls back to plain login] over-length x-pathname redirects to plain /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    headerValues.pathname = `/seller/${"a".repeat(2100)}`;
    await expect(SellerLayout({ children: null })).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("[Signed-in user without the scope is not redirected to login] buyer sees upgrade prompt not login", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...seller, scopes: [] });
    headerValues.pathname = "/seller/new";
    await renderLayout();
    expect(screen.getByText("Cần tài khoản Người Bán")).toBeInTheDocument();
    expect(redirect).not.toHaveBeenCalled();
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
