import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/auth/actions", () => ({ logoutAction: vi.fn() }));

import { AdminShell } from "./AdminShell";
import { ADMIN_NAV, visibleNav } from "./nav";

describe("AdminShell", () => {
  it("links the ready admin screens", () => {
    render(<AdminShell userName="root">x</AdminShell>);
    const nav = screen.getByRole("navigation", { name: "Quản trị" });
    const hrefs = within(nav)
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual([
      "/admin/cockpit",
      "/admin/listings",
      "/admin/cases",
      "/admin/users",
      "/admin/audit",
      "/admin/campaigns",
      "/admin/plans",
    ]);
  });

  it("hides the voucher placeholder until it exists", () => {
    expect(ADMIN_NAV.filter((i) => !i.ready).map((i) => i.href)).toEqual([
      "/admin/vouchers",
    ]);
    expect(visibleNav().some((i) => i.href === "/admin/vouchers")).toBe(false);
  });

  it("shows the user and a logout button", () => {
    render(<AdminShell userName="root">x</AdminShell>);
    expect(screen.getByTestId("admin-user")).toHaveTextContent("root");
    expect(screen.getByRole("button", { name: "Đăng xuất" })).toBeVisible();
  });
});
