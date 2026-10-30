import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal, getToken } from "@/lib/gateway/session";

import NewVoucherPage from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/features/vouchers/actions", () => ({
  createPlatformVoucherAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getToken).mockReturnValue("tok");
});

describe("NewVoucherPage", () => {
  it("renders the create form for an admin", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "a",
      name: "A",
      scopes: ["admin"],
    });
    render(await NewVoucherPage());
    expect(
      screen.getByRole("heading", { name: "Tạo voucher sàn" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("voucher-create-form")).toBeInTheDocument();
  });

  it("shows 403 without the admin scope", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ id: "b", name: "B", scopes: [] });
    render(await NewVoucherPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(screen.queryByTestId("voucher-create-form")).toBeNull();
  });

  it("redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await NewVoucherPage()).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/vouchers/new")}`,
    );
  });
});
