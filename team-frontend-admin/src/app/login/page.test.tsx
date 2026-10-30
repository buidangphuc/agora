import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/auth/actions", () => ({ loginAction: vi.fn() }));

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
