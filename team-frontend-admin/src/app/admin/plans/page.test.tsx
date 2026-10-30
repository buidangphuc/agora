import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listPlans } from "@/lib/gateway/campaigns";
import { getPrincipal, getToken } from "@/lib/gateway/session";

import PlansPage, { metadata } from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/campaigns", () => ({ listPlans: vi.fn() }));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "u-admin",
    name: "Admin",
    scopes: ["admin"],
  });
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listPlans).mockResolvedValue({
    status: "ok",
    plans: [{ id: "p-pro", tier: "pro", price: 199000, features: ["a"] }],
  });
});

describe("PlansPage", () => {
  it("declares no-referrer metadata", () => {
    expect(metadata.referrer).toBe("no-referrer");
  });

  it("redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await PlansPage()).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/plans")}`,
    );
    expect(listPlans).not.toHaveBeenCalled();
  });

  it("shows 403 to a non-admin", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-2",
      name: "Buyer",
      scopes: ["order.read"],
    });
    render(await PlansPage());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(listPlans).not.toHaveBeenCalled();
  });

  it("renders the plans", async () => {
    render(await PlansPage());
    expect(screen.getAllByTestId("plans-row")).toHaveLength(1);
  });

  it("shows the load error", async () => {
    vi.mocked(listPlans).mockResolvedValue({ status: "unavailable" });
    render(await PlansPage());
    expect(
      screen.getByText(
        "Không tải được danh sách gói đăng ký. Vui lòng thử lại sau.",
      ),
    ).toBeInTheDocument();
  });
});
