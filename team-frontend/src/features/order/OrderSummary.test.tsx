import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { setupUser } from "@/test/user";

import { OrderSummary } from "./OrderSummary";

const action = <button type="button">Đi tiếp</button>;

describe("OrderSummary", () => {
  it("shows subtotal, discount, shipping and total = max(0, subtotal - discount + shipping)", () => {
    render(
      <OrderSummary
        itemCount={2}
        subtotal={300000}
        discount={30000}
        voucherCode="SAVE10"
        shipping={{ fee: 20000, isFree: false }}
        action={action}
      />,
    );
    const card = screen.getByTestId("order-summary");
    expect(within(card).getByText("₫300.000")).toBeInTheDocument();
    expect(within(card).getByTestId("voucher-discount")).toHaveTextContent(
      "-₫30.000",
    );
    expect(within(card).getByText("₫20.000")).toBeInTheDocument();
    expect(within(card).getByTestId("order-total")).toHaveTextContent(
      "290.000",
    );
  });

  it("always renders the discount row ('-' when none) and a Freeship tag", () => {
    render(
      <OrderSummary
        itemCount={1}
        subtotal={600000}
        discount={0}
        shipping={{ fee: 0, isFree: true }}
        action={action}
      />,
    );
    const card = screen.getByTestId("order-summary");
    expect(within(card).getByTestId("voucher-discount")).toHaveTextContent("-");
    expect(within(card).getByText("Freeship")).toBeInTheDocument();
    expect(within(card).getByTestId("order-total")).toHaveTextContent(
      "600.000",
    );
  });

  it("shows the fee as pending when no address is chosen yet", () => {
    render(
      <OrderSummary
        itemCount={1}
        subtotal={100000}
        discount={0}
        shipping={null}
        action={action}
      />,
    );
    expect(screen.getByText("Tính ở bước thanh toán")).toBeInTheDocument();
  });

  it("mobile: sticky bottom bar with the total opens a Drawer with the breakdown", async () => {
    const user = setupUser();
    render(
      <OrderSummary
        itemCount={1}
        subtotal={100000}
        discount={0}
        shipping={{ fee: 35000, isFree: false }}
        action={action}
        mobileAction={<button type="button">Tiếp tục</button>}
      />,
    );
    const bar = screen.getByTestId("mobile-summary-bar");
    expect(bar.className).toContain("lg:hidden");
    expect(bar.className).toContain("fixed");
    expect(within(bar).getByText("135.000")).toBeInTheDocument();
    expect(within(bar).getByRole("button", { name: "Tiếp tục" })).toBeVisible();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await user.click(within(bar).getByRole("button", { name: "Chi tiết" }));
    const drawer = screen.getByRole("dialog");
    expect(within(drawer).getByText("₫35.000")).toBeInTheDocument();
    expect(within(drawer).getByText(/Tạm tính/)).toBeInTheDocument();
  });

  it("hides the desktop card below lg (no horizontal overflow source) and keeps it sticky from lg", () => {
    render(
      <OrderSummary
        itemCount={1}
        subtotal={1}
        discount={0}
        shipping={null}
        action={action}
      />,
    );
    const card = screen.getByTestId("order-summary");
    expect(card.className).toContain("hidden");
    expect(card.className).toContain("lg:sticky");
    expect(card.className).toContain("lg:block");
  });

  it("keeps the discount label identical with and without a voucher (no layout shift)", () => {
    const props = {
      itemCount: 1,
      subtotal: 100000,
      shipping: { fee: 0, isFree: true },
      action,
    };
    const { rerender } = render(<OrderSummary {...props} discount={0} />);
    const dtText = () =>
      within(screen.getByTestId("order-summary"))
        .getAllByText("Giảm giá")
        .map((el) => el.closest("dt")?.textContent);
    expect(dtText()).toEqual(["Giảm giá"]);
    rerender(<OrderSummary {...props} discount={10000} voucherCode="SAVE10" />);
    const card = screen.getByTestId("order-summary");
    // chip lives in the value (fixed-height line), never in the wrapping label
    expect(dtText()).toEqual(["Giảm giá"]);
    expect(within(card).getByText("SAVE10").closest("dt")).toBeNull();
    expect(within(card).getByTestId("voucher-discount")).toHaveTextContent(
      "-₫10.000",
    );
  });
});
