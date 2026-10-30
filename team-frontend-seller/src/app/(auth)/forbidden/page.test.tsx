import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/auth/actions", () => ({ logoutAction: vi.fn() }));

import ForbiddenPage from "./page";

describe("seller forbidden page", () => {
  it("names the missing seller access and keeps the restricted notice", () => {
    render(<ForbiddenPage />);
    expect(screen.getByText("Không có quyền")).toBeInTheDocument();
    expect(screen.getByText("Cần tài khoản Người Bán")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người bán cho đến khi tài khoản được khôi phục.",
      ),
    ).toBeInTheDocument();
  });

  it("links to seller onboarding in this app and back to the buyer app", () => {
    render(<ForbiddenPage />);
    expect(
      screen.getByRole("link", { name: "Đăng ký bán hàng" }),
    ).toHaveAttribute("href", "/sell");
    expect(
      screen.getByRole("link", { name: "Về trang mua sắm" }),
    ).toHaveAttribute("href", "http://localhost:3000/");
  });

  it("lets the user sign out of the seller app to switch account", () => {
    render(<ForbiddenPage />);
    expect(screen.getByRole("button", { name: "Đăng xuất" })).toHaveAttribute(
      "type",
      "submit",
    );
  });
});
