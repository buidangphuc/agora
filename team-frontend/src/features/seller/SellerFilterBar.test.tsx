import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FILTER_DEBOUNCE_MS, SellerFilterBar } from "./SellerFilterBar";

const replace = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
  usePathname: () => "/seller",
}));

beforeEach(() => {
  vi.useFakeTimers();
  replace.mockClear();
});
afterEach(() => vi.useRealTimers());

describe("SellerFilterBar", () => {
  it("debounces typing into a single router.replace with the page reset", () => {
    render(<SellerFilterBar basePath="/seller" q="" status="published" />);
    const box = screen.getByRole("searchbox");
    fireEvent.change(box, { target: { value: "i" } });
    fireEvent.change(box, { target: { value: "iphone" } });
    expect(replace).not.toHaveBeenCalled();
    act(() => {
      vi.advanceTimersByTime(FILTER_DEBOUNCE_MS + 10);
    });
    expect(replace).toHaveBeenCalledTimes(1);
    expect(replace).toHaveBeenCalledWith("/seller?q=iphone&status=published");
  });

  it("clearing the search drops q and keeps fixed params", () => {
    render(
      <SellerFilterBar
        basePath="/seller/orders"
        q="abc"
        fixed={{ status: "shipped" }}
      />,
    );
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "" } });
    act(() => {
      vi.advanceTimersByTime(FILTER_DEBOUNCE_MS + 10);
    });
    expect(replace).toHaveBeenCalledWith("/seller/orders?status=shipped");
  });

  it("a status change navigates immediately", () => {
    render(
      <SellerFilterBar
        basePath="/seller"
        q="x"
        status="all"
        statusOptions={[
          { value: "all", label: "Tất cả" },
          { value: "draft", label: "Bản nháp" },
        ]}
      />,
    );
    fireEvent.change(screen.getByLabelText("Trạng thái"), {
      target: { value: "draft" },
    });
    expect(replace).toHaveBeenCalledWith("/seller?q=x&status=draft");
  });
});
