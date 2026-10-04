import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { setupUser } from "@/test/user";

import { PaymentOptionsGrid } from "./PaymentOptionsGrid";

const nav = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: nav.replace }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams("step=payment&addr=a1"),
}));

beforeEach(() => vi.clearAllMocks());

describe("PaymentOptionsGrid", () => {
  it("renders four methods, exactly one checked, COD by default", () => {
    render(<PaymentOptionsGrid selected={PaymentMethod.COD} />);
    const radios = screen.getAllByRole("radio");
    expect(radios).toHaveLength(4);
    expect(radios.filter((r) => (r as HTMLInputElement).checked)).toHaveLength(
      1,
    );
    expect(
      screen.getByRole("radio", { name: /Thanh toán khi nhận hàng/ }),
    ).toBeChecked();
  });

  it("selects the next method with the Down arrow and mirrors it to ?pay=", async () => {
    const user = setupUser();
    render(<PaymentOptionsGrid selected={PaymentMethod.COD} />);
    const cod = screen.getByRole("radio", { name: /Thanh toán khi nhận hàng/ });
    cod.focus();
    await user.keyboard("{ArrowDown}");

    const radios = screen.getAllByRole("radio") as HTMLInputElement[];
    expect(radios.filter((r) => r.checked)).toHaveLength(1);
    expect(screen.getByRole("radio", { name: /MoMo/ })).toBeChecked();
    expect(nav.replace).toHaveBeenLastCalledWith(
      `/checkout?step=payment&addr=a1&pay=${PaymentMethod.MOCK_MOMO}`,
    );
  });

  it("uses one column on mobile and keeps each card at least 44px tall", () => {
    const { container } = render(
      <PaymentOptionsGrid selected={PaymentMethod.COD} />,
    );
    const grid = container.querySelector(".grid") as HTMLElement;
    expect(grid.className).toContain("grid-cols-1");
    expect(grid.className).toContain("sm:grid-cols-2");
    for (const card of container.querySelectorAll("[data-selected]")) {
      expect(card.className).toContain("min-h-12");
    }
  });
});
