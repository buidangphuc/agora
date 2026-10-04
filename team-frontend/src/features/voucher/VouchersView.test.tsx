import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  DiscountType,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { ViewVoucher } from "@/lib/gateway/promotion";

import { VoucherCard } from "./VoucherCard";
import { VouchersView } from "./VouchersView";
import { inTab, parseVoucherTab, tabCounts, voucherTabHref } from "./tabs";

function voucher(over: Partial<ViewVoucher> = {}): ViewVoucher {
  return {
    id: "v1",
    code: "SAVE10",
    scope: VoucherScope.PLATFORM,
    scopeText: "Toàn sàn",
    sellerId: "",
    discountType: DiscountType.PERCENT,
    discountTypeText: "Giảm %",
    discountValue: 10,
    minSpend: 0,
    maxDiscount: 50000,
    quota: 100,
    used: 25,
    startsAt: "01/10/2026",
    endsAt: "31/10/2026",
    ...over,
  };
}

const list = [
  voucher(),
  voucher({
    id: "v2",
    code: "MINUS50K",
    discountType: DiscountType.FIXED,
    discountTypeText: "Giảm tiền",
    discountValue: 50000,
    minSpend: 200000,
    quota: 0,
    maxDiscount: 0,
  }),
  voucher({ id: "v3", code: "SUPER5", discountValue: 5 }),
];

describe("tabs", () => {
  it("parses the type with an all fallback", () => {
    expect(parseVoucherTab(undefined)).toBe("all");
    expect(parseVoucherTab("fixed")).toBe("fixed");
    expect(parseVoucherTab("nope")).toBe("all");
    expect(parseVoucherTab(["percent", "x"])).toBe("percent");
  });

  it("counts per tab; shipping matches nothing without a backend type", () => {
    expect(tabCounts(list)).toEqual({
      all: 3,
      shipping: 0,
      fixed: 1,
      percent: 2,
    });
    expect(inTab(list[0] as ViewVoucher, "shipping")).toBe(false);
  });

  it("builds tab URLs", () => {
    expect(voucherTabHref("all")).toBe("/vouchers");
    expect(voucherTabHref("shipping")).toBe("/vouchers?type=shipping");
  });
});

describe("VouchersView", () => {
  it("filters cards by the selected tab and shows counts", () => {
    render(<VouchersView vouchers={list} type="fixed" />);
    expect(screen.getAllByTestId("voucher-card")).toHaveLength(1);
    expect(screen.getByTestId("voucher-card")).toHaveAttribute(
      "data-code",
      "MINUS50K",
    );
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: /Tất cả/ }),
    ).toHaveTextContent("3");
    expect(
      within(tabs).getByRole("link", { name: /Giảm %/ }),
    ).toHaveTextContent("2");
    expect(
      within(tabs).getByRole("link", { name: /Giảm tiền/ }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      within(tabs).getByRole("link", { name: /Freeship/ }),
    ).toHaveAttribute("href", "/vouchers?type=shipping");
  });

  it("shows Empty with a way to products for an empty tab", () => {
    render(<VouchersView vouchers={list} type="shipping" />);
    expect(screen.queryByTestId("voucher-card")).toBeNull();
    expect(
      screen.getByText("Chưa có voucher nào trong mục này."),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem sản phẩm" })).toHaveAttribute(
      "href",
      "/search",
    );
  });

  it("shows Empty when there are no vouchers at all (also the outage case)", () => {
    render(<VouchersView vouchers={[]} />);
    expect(
      screen.getByRole("link", { name: "Xem sản phẩm" }),
    ).toBeInTheDocument();
  });

  it("has no 'Lưu mã' / 'Đã lưu' control and never touches localStorage", () => {
    const get = vi.spyOn(Storage.prototype, "getItem");
    const set = vi.spyOn(Storage.prototype, "setItem");
    const { container } = render(<VouchersView vouchers={list} />);
    expect(container.textContent).not.toMatch(/Lưu mã|Đã lưu/);
    expect(screen.queryByRole("button")).toBeNull();
    expect(get).not.toHaveBeenCalled();
    expect(set).not.toHaveBeenCalled();
  });

  it("lays out one column at 375px and two from lg, with scrollable tabs", () => {
    const { container } = render(<VouchersView vouchers={list} />);
    const grid = container.querySelector(".grid") as HTMLElement;
    expect(grid).toHaveClass("grid-cols-1", "lg:grid-cols-2");
    expect(container.querySelector("nav[aria-label='Tabs'] ul")).toHaveClass(
      "overflow-x-auto",
    );
  });

  it("route sources never read browser storage or the static voucher list", () => {
    const files = [
      "src/features/voucher/VouchersView.tsx",
      "src/features/voucher/VoucherCard.tsx",
      "src/features/voucher/VoucherManager.tsx",
      "src/app/(shop)/vouchers/page.tsx",
      ...readdirSync(resolve(process.cwd(), "src/app/(shop)/vouchers"))
        .filter((f) => f.endsWith(".tsx") && !f.includes(".test."))
        .map((f) => `src/app/(shop)/vouchers/${f}`),
    ];
    for (const f of files) {
      const src = readFileSync(resolve(process.cwd(), f), "utf8");
      expect(src, f).not.toMatch(
        /localStorage|sessionStorage|AVAILABLE_VOUCHERS/,
      );
    }
  });
});

describe("VoucherCard", () => {
  it("shows real figures: code, discount, min spend, expiry, usage, and Dùng ngay", () => {
    render(<VoucherCard voucher={voucher({ minSpend: 200000 })} />);
    expect(screen.getByText("SAVE10")).toBeInTheDocument();
    expect(screen.getByText(/Giảm 10% tối đa/)).toBeInTheDocument();
    expect(screen.getByText(/Cho đơn từ/)).toBeInTheDocument();
    expect(screen.getByText("HSD: 31/10/2026")).toBeInTheDocument();
    expect(screen.getByText("Đã dùng 25/100")).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { hidden: true })).toHaveAttribute(
      "value",
      "25",
    );
    expect(screen.getByRole("link", { name: "Dùng ngay" })).toHaveAttribute(
      "href",
      "/search",
    );
  });

  it("shows no usage bar for an unlimited quota", () => {
    render(<VoucherCard voucher={voucher({ quota: 0, used: 7 })} />);
    expect(screen.queryByText(/Đã dùng/)).toBeNull();
    expect(document.querySelector("progress")).toBeNull();
  });
});
