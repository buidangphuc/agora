import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PlansView } from "./PlansView";

describe("PlansView", () => {
  it("lists plans with tier, price and features", () => {
    render(
      <PlansView
        plans={[
          { id: "p-free", tier: "free", price: 0, features: [] },
          {
            id: "p-pro",
            tier: "pro",
            price: 199000,
            features: ["10 tin nổi bật"],
          },
        ]}
      />,
    );
    const rows = screen.getAllByTestId("plans-row");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Miễn phí");
    expect(rows[0]).toHaveTextContent("Không có");
    expect(rows[1]).toHaveTextContent("Pro");
    expect(rows[1]).toHaveTextContent("10 tin nổi bật");
    expect(screen.getByTestId("plans-gap")).toBeInTheDocument();
  });

  it("shows the load error", () => {
    render(<PlansView plans={[]} error="unavailable" />);
    expect(
      screen.getByText(
        "Không tải được danh sách gói đăng ký. Vui lòng thử lại sau.",
      ),
    ).toBeInTheDocument();
  });
});
