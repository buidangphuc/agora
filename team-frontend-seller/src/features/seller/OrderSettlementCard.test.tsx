import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { formatPrice } from "@/components/ui/format";
import type { ViewOrderSettlement } from "@/lib/gateway/payment";

import { OrderSettlementCard } from "./OrderSettlementCard";

const retryHref = "/seller/orders/o1";

function settled(over: Partial<ViewOrderSettlement> = {}): ViewOrderSettlement {
  return {
    credited: 310000,
    commission: 14000,
    commissionRatePercent: 5,
    commissionReversed: 0,
    refundDeducted: 0,
    net: 296000,
    ...over,
  };
}

describe("OrderSettlementCard", () => {
  it("Breakdown of a settled order: renders credited, commission with rate, net and zero refunds", () => {
    render(
      <OrderSettlementCard settlement={settled()} retryHref={retryHref} />,
    );
    expect(screen.getByText("Đối soát thanh toán")).toBeInTheDocument();
    expect(screen.getByText(formatPrice(310000))).toBeInTheDocument();
    expect(screen.getByText(formatPrice(296000))).toBeInTheDocument();
    expect(screen.getByText(/5%/)).toBeInTheDocument();
    expect(
      screen.getByText(new RegExp(`^${formatPrice(14000)}`)),
    ).toBeInTheDocument();
    expect(screen.getAllByText(formatPrice(0))).toHaveLength(2);
    expect(screen.queryByText("Đơn chưa được đối soát vào ví")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("Breakdown after a partial refund: renders commission returned, refunds deducted and net", () => {
    render(
      <OrderSettlementCard
        settlement={settled({
          commissionReversed: 5600,
          refundDeducted: 124000,
          net: 177600,
        })}
        retryHref={retryHref}
      />,
    );
    expect(screen.getByText(formatPrice(5600))).toBeInTheDocument();
    expect(screen.getByText(formatPrice(124000))).toBeInTheDocument();
    expect(screen.getByText(formatPrice(177600))).toBeInTheDocument();
  });

  it("Zero commission is shown, not hidden: renders 0% and a zero amount", () => {
    render(
      <OrderSettlementCard
        settlement={settled({
          commission: 0,
          commissionRatePercent: 0,
          commissionReversed: 1000,
          refundDeducted: 2000,
          net: 309000,
        })}
        retryHref={retryHref}
      />,
    );
    expect(screen.getByText(/0%/)).toBeInTheDocument();
    expect(
      screen.getByText(new RegExp(`^${formatPrice(0)}`)),
    ).toBeInTheDocument();
    expect(screen.getByText(formatPrice(310000))).toBeInTheDocument();
    expect(screen.getByText(formatPrice(309000))).toBeInTheDocument();
  });

  it("Unsettled order shows the not-settled state: message and no amount rows", () => {
    const { container } = render(
      <OrderSettlementCard settlement={null} retryHref={retryHref} />,
    );
    expect(screen.getByText("Đối soát thanh toán")).toBeInTheDocument();
    expect(
      screen.getByText("Đơn chưa được đối soát vào ví"),
    ).toBeInTheDocument();
    expect(container.textContent).not.toContain("₫");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("Settlement service failure degrades only the card: error alert with retry link", () => {
    const { container } = render(
      <OrderSettlementCard settlement={null} failed retryHref={retryHref} />,
    );
    const alert = screen.getByRole("alert");
    expect(
      within(alert).getByText("Không tải được đối soát thanh toán"),
    ).toBeInTheDocument();
    expect(
      within(alert).getByRole("link", { name: "Thử lại" }),
    ).toHaveAttribute("href", retryHref);
    expect(screen.queryByText("Đơn chưa được đối soát vào ví")).toBeNull();
    expect(container.textContent).not.toContain("₫");
  });
});
