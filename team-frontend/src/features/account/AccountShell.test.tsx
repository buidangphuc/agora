import { render, screen, within } from "@testing-library/react";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it, vi } from "vitest";

import { ACCOUNT_MENU, AccountShell } from "./AccountShell";

vi.mock("@/features/welcome/WelcomePanel", () => ({
  WelcomePanel: () => <p>WELCOME_PANEL</p>,
}));

describe("AccountShell", () => {
  it("labels the menu landmark and marks only the current entry", () => {
    render(
      <AccountShell current="security" title="Bảo mật tài khoản">
        <p>nội dung</p>
      </AccountShell>,
    );
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    const links = within(nav).getAllByRole("link");
    expect(links).toHaveLength(ACCOUNT_MENU.length);

    const current = within(nav).getByRole("link", { name: "Bảo mật" });
    expect(current).toHaveAttribute("aria-current", "page");
    for (const link of links) {
      if (link !== current) expect(link).not.toHaveAttribute("aria-current");
    }
  });

  it("renders the h1, subtitle, breadcrumb and children", () => {
    render(
      <AccountShell
        current="security"
        title="Bảo mật tài khoản"
        description="Gửi giấy tờ để xác minh."
      >
        <p>nội dung</p>
      </AccountShell>,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Bảo mật tài khoản" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Gửi giấy tờ để xác minh.")).toBeInTheDocument();
    const crumbs = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(crumbs).getByText("Tài khoản")).toBeInTheDocument();
    expect(within(crumbs).getByText("Bảo mật")).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("nội dung")).toBeInTheDocument();
  });

  it("menu entries are real links to their routes", () => {
    render(
      <AccountShell current="addresses" title="Địa chỉ">
        x
      </AccountShell>,
    );
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    expect(within(nav).getByRole("link", { name: "Đơn hàng" })).toHaveAttribute(
      "href",
      "/account/orders",
    );
    expect(
      within(nav).getByRole("link", { name: "Thông báo" }),
    ).toHaveAttribute("href", "/notifications");
  });

  it("scrolls the menu row on mobile and becomes a 240px column from lg", () => {
    render(
      <AccountShell current="addresses" title="Địa chỉ">
        x
      </AccountShell>,
    );
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    expect(nav.className).toContain("lg:w-60");
    expect(nav.querySelector("ul")?.className).toContain("overflow-x-auto");
  });
});

describe("AccountShell privacy entry", () => {
  it("PV1 lists the privacy entry and marks it current", () => {
    render(
      <AccountShell current="privacy" title="Quyền riêng tư">
        x
      </AccountShell>,
    );
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    const link = within(nav).getByRole("link", { name: "Quyền riêng tư" });
    expect(link).toHaveAttribute("href", "/account/privacy");
    expect(link).toHaveAttribute("aria-current", "page");
  });
});

describe("AccountShell welcome panel", () => {
  it("W10 mounts the WelcomePanel above the page content on every account page", () => {
    render(
      <AccountShell current="addresses" title="Địa chỉ">
        <p>nội dung</p>
      </AccountShell>,
    );
    const panel = screen.getByText("WELCOME_PANEL");
    expect(
      panel.compareDocumentPosition(screen.getByText("nội dung")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("W9 mounts the WelcomePanel exactly once", () => {
    render(
      <AccountShell current="security" title="Bảo mật">
        <p>x</p>
      </AccountShell>,
    );
    expect(screen.getAllByText("WELCOME_PANEL")).toHaveLength(1);
  });
});

describe("seller-kyc-ui buyer verification removal", () => {
  it("[Buyer navigation has no verification link] the account menu has 0 entries to /account/verification", () => {
    expect(ACCOUNT_MENU.filter((m) => m.href.includes("verification"))).toHaveLength(0);
    render(
      <AccountShell current="addresses" title="Địa chỉ">
        x
      </AccountShell>,
    );
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    const hrefs = within(nav)
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"));
    expect(hrefs).not.toContain("/account/verification");
    expect(within(nav).queryByRole("link", { name: "Xác minh" })).toBeNull();
  });

  it("[The buyer route is gone] no /account/verification route or feature folder remains", () => {
    const src = join(__dirname, "..", "..");
    expect(existsSync(join(src, "app/(shop)/account/verification"))).toBe(false);
    expect(existsSync(join(src, "features/account/verification"))).toBe(false);
  });
});
