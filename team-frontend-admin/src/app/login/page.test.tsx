import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";

vi.mock("@/features/auth/actions", () => ({ loginAction: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn(() => null) }));

beforeEach(() => {
  vi.mocked(redirect).mockClear();
  vi.mocked(getPrincipal).mockReturnValue(null);
});

function returnUrlOf(container: HTMLElement) {
  return (
    container.querySelector('input[name="returnUrl"]')?.getAttribute("value") ??
    ""
  );
}

import LoginPage from "./page";

describe("admin login page", () => {
  it("renders the login form without a registration link", async () => {
    render(await LoginPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByRole("heading", { name: "Đăng nhập" })).toBeVisible();
    expect(screen.getByLabelText(/Tên đăng nhập/)).toBeVisible();
    expect(screen.queryByRole("link", { name: /Đăng ký/ })).toBeNull();
  });

  it("keeps a safe return url", async () => {
    const { container } = render(
      await LoginPage({
        searchParams: Promise.resolve({ returnUrl: "/admin/users" }),
      }),
    );
    expect(
      container.querySelector('input[name="returnUrl"]')?.getAttribute("value"),
    ).toBe("/admin/users");
  });
});

describe("admin login page next param", () => {
  const admin = { id: "a1", name: "root", scopes: ["admin"] };

  it("keeps a safe next and prefers it over returnUrl", async () => {
    const { container } = render(
      await LoginPage({
        searchParams: Promise.resolve({
          next: "/admin/audit",
          returnUrl: "/admin/users",
        }),
      }),
    );
    expect(returnUrlOf(container)).toBe("/admin/audit");
  });

  it("drops an unsafe next", async () => {
    const { container } = render(
      await LoginPage({
        searchParams: Promise.resolve({ next: "https://evil.example" }),
      }),
    );
    expect(returnUrlOf(container)).not.toContain("evil");
  });

  it("sends a signed-in admin straight to next", async () => {
    vi.mocked(getPrincipal).mockReturnValue(admin);
    await LoginPage({
      searchParams: Promise.resolve({ next: "/admin/users" }),
    });
    expect(redirect).toHaveBeenCalledWith("/admin/users");
  });

  it("sends a signed-in admin without next to the cockpit", async () => {
    vi.mocked(getPrincipal).mockReturnValue(admin);
    await LoginPage({ searchParams: Promise.resolve({}) });
    expect(redirect).toHaveBeenCalledWith("/admin/cockpit");
  });

  it("shows the form to a signed-in user without the admin scope", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...admin, scopes: [] });
    render(await LoginPage({ searchParams: Promise.resolve({}) }));
    expect(redirect).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "Đăng nhập" })).toBeVisible();
  });
});
