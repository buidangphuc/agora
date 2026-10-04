import { render, screen, within } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listAddresses } from "@/lib/gateway/addresses";
import { getPrincipal } from "@/lib/gateway/session";

vi.mock("@/lib/gateway/addresses", () => ({ listAddresses: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/features/address/actions", () => ({
  createAddressAction: vi.fn(),
  updateAddressAction: vi.fn(),
  deleteAddressAction: vi.fn(),
  setDefaultAddressAction: vi.fn(),
}));

import AccountAddressesPage from "./page";

beforeEach(() => vi.clearAllMocks());

describe("/account/addresses", () => {
  it("redirects an anonymous visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(listAddresses).mockResolvedValue([]);
    await AccountAddressesPage();
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("renders inside the AccountShell with the address list", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ name: "u" } as never);
    vi.mocked(listAddresses).mockResolvedValue([
      {
        id: "a1",
        userId: "u",
        recipientName: "Nguyễn Văn A",
        phone: "0912345678",
        street: "1 Lê Lợi",
        ward: "",
        district: "",
        city: "TP. HCM",
        isDefault: true,
      },
    ]);
    render(await AccountAddressesPage());
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    expect(within(nav).getByRole("link", { name: "Địa chỉ" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("Nguyễn Văn A")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Thêm địa chỉ mới" }),
    ).toBeVisible();
  });
});
