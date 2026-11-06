import { render, screen, within } from "@testing-library/react";
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
        current="verification"
        title="Xác minh tài khoản"
        description="Gửi giấy tờ để xác minh."
      >
        <p>nội dung</p>
      </AccountShell>,
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Xác minh tài khoản" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Gửi giấy tờ để xác minh.")).toBeInTheDocument();
    const crumbs = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(within(crumbs).getByText("Tài khoản")).toBeInTheDocument();
    expect(within(crumbs).getByText("Xác minh")).toHaveAttribute(
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
    expect(within(nav).getByRole("link", { name: "Xác minh" })).toHaveAttribute(
      "href",
      "/account/verification",
    );
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
