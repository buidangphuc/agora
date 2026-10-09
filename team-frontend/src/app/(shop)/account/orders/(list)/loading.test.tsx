import { render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import OrdersError from "../error";
import OrdersLoading from "./loading";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));

describe("orders list route states", () => {
  it("loading renders a busy skeleton of the tab bar and three cards", () => {
    const { container } = render(<OrdersLoading />);
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(screen.getAllByText("Đang tải").length).toBeGreaterThanOrEqual(3);
  });

  it("error renders an Alert whose retry calls reset", async () => {
    const reset = vi.fn();
    render(<OrdersError error={new Error("x")} reset={reset} />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
    await setupUser().click(screen.getByRole("button", { name: "Thử lại" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });
});
