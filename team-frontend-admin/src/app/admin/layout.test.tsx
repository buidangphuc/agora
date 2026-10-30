import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const getPrincipal = vi.hoisted(() => vi.fn());
const redirectGuest = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/session", () => ({ getPrincipal }));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLoginFromHeader: redirectGuest,
}));
vi.mock("@/features/auth/actions", () => ({ logoutAction: vi.fn() }));

import AdminLayout from "./layout";

beforeEach(() => {
  getPrincipal.mockReset();
  redirectGuest.mockReset();
});

describe("admin layout gate", () => {
  it("sends a guest to the admin login", async () => {
    getPrincipal.mockReturnValue(null);
    await AdminLayout({ children: "secret" });
    expect(redirectGuest).toHaveBeenCalled();
  });

  it("shows Không có quyền to a signed-in user without the admin scope", async () => {
    getPrincipal.mockReturnValue({
      id: "u1",
      name: "b",
      scopes: ["listing.read"],
    });
    render((await AdminLayout({ children: "secret" })) as ReactElement);
    expect(
      screen.getByRole("heading", { name: "Không có quyền" }),
    ).toBeVisible();
    expect(screen.queryByText("secret")).toBeNull();
    expect(redirectGuest).not.toHaveBeenCalled();
  });

  it("renders the shell and the page for an admin", async () => {
    getPrincipal.mockReturnValue({ id: "u1", name: "root", scopes: ["admin"] });
    render((await AdminLayout({ children: "secret" })) as ReactElement);
    expect(redirectGuest).not.toHaveBeenCalled();
    expect(screen.getByText("secret")).toBeVisible();
    expect(screen.getByRole("navigation", { name: "Quản trị" })).toBeVisible();
  });
});
