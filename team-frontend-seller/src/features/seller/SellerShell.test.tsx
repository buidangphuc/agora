import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SellerNavDrawer } from "./SellerNavDrawer";
import { SIDEBAR_STORAGE_KEY, SellerSidebar } from "./SellerSidebar";

let pathname = "/seller";
vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
  redirect: vi.fn(),
  notFound: vi.fn(),
}));

const shop = { sellerId: "seller-123456", name: "Cửa hàng Hoa Mai" };

beforeEach(() => {
  pathname = "/seller";
  window.localStorage.clear();
});

describe("SellerSidebar", () => {
  it("collapses to 64px, keeps accessible link names and remembers the choice", () => {
    render(<SellerSidebar shop={shop} />);
    const aside = screen.getByRole("complementary");
    expect(aside).toHaveClass("w-64");

    fireEvent.click(screen.getByRole("button", { name: "Thu gọn thanh bên" }));
    expect(aside).toHaveClass("w-16");
    expect(aside).toHaveAttribute("data-collapsed", "true");
    // The label is visually hidden, not removed: the link keeps its name.
    expect(
      screen.getByRole("link", { name: "Quản lý đơn hàng" }),
    ).toHaveAttribute("href", "/seller/orders");
    expect(window.localStorage.getItem(SIDEBAR_STORAGE_KEY)).toBe("1");

    fireEvent.click(screen.getByRole("button", { name: "Mở rộng thanh bên" }));
    expect(aside).toHaveClass("w-64");
  });

  it("starts collapsed when the stored preference says so", () => {
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, "1");
    render(<SellerSidebar shop={shop} />);
    expect(screen.getByRole("complementary")).toHaveClass("w-16");
  });

  it("marks only the current route with aria-current=page", () => {
    pathname = "/seller/wallet";
    render(<SellerSidebar shop={shop} />);
    const current = screen
      .getAllByRole("link")
      .filter((a) => a.getAttribute("aria-current") === "page");
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveTextContent("Ví người bán");
  });

  it("treats the edit route as part of the product list", () => {
    pathname = "/seller/abc/edit";
    render(<SellerSidebar shop={shop} />);
    expect(
      screen.getByRole("link", { name: "Tất cả sản phẩm" }),
    ).toHaveAttribute("aria-current", "page");
  });

  it("shows the shop name from props", () => {
    render(<SellerSidebar shop={shop} />);
    expect(screen.getByTestId("seller-shop-name")).toHaveTextContent(
      "Cửa hàng Hoa Mai",
    );
  });
});

describe("SellerNavDrawer", () => {
  it("opens the navigation in a dialog and closes it on navigation", () => {
    const { rerender } = render(<SellerNavDrawer shop={shop} />);
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Mở menu người bán" }));
    const dialog = screen.getByRole("dialog");
    // jsdom cannot navigate; swallow the default so only React's handler runs.
    document.addEventListener("click", (e) => e.preventDefault(), {
      once: true,
    });
    fireEvent.click(
      within(dialog).getByRole("link", { name: "Quản lý đơn hàng" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();

    // Reopen, then a route change (pathname) also closes it.
    fireEvent.click(screen.getByRole("button", { name: "Mở menu người bán" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    pathname = "/seller/orders";
    rerender(<SellerNavDrawer shop={shop} />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes on Escape", () => {
    render(<SellerNavDrawer shop={shop} />);
    fireEvent.click(screen.getByRole("button", { name: "Mở menu người bán" }));
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
