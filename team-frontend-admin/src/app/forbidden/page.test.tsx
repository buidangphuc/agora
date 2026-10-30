import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/auth/actions", () => ({ logoutAction: vi.fn() }));

import ForbiddenPage from "./page";

describe("forbidden page", () => {
  it("says the account has no admin permission and offers logout", () => {
    render(<ForbiddenPage />);
    expect(
      screen.getByRole("heading", { name: "Không có quyền" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Đăng xuất" })).toBeVisible();
  });
});
