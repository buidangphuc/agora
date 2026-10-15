import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce, checkInAction, toast } = vi.hoisted(() => ({
  trackEcommerce: vi.fn(),
  checkInAction: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("./actions", () => ({ checkInAction }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { LoyaltyWidget } from "./LoyaltyWidget";

const initial = { streak: 2, coinBalance: 10 } as never;
const result = (streak: number) => ({
  ok: true,
  result: { streak, coinBalance: 20, coinsEarned: 5 },
});

beforeEach(() => vi.clearAllMocks());

describe("LoyaltyWidget loop events", () => {
  it("tracks daily_check_in with streak", async () => {
    checkInAction.mockResolvedValue(result(3));
    render(<LoyaltyWidget initial={initial} />);
    fireEvent.click(screen.getByRole("button", { name: "Điểm danh" }));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce).toHaveBeenCalledWith("daily_check_in", {
      properties: { streak: "3" },
    });
  });

  it("omits streak when absent", async () => {
    checkInAction.mockResolvedValue(result(0));
    render(<LoyaltyWidget initial={initial} />);
    fireEvent.click(screen.getByRole("button", { name: "Điểm danh" }));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce).toHaveBeenCalledWith("daily_check_in", {});
  });

  it("tracks nothing on failure", async () => {
    checkInAction.mockResolvedValue({ ok: false, message: "already" });
    render(<LoyaltyWidget initial={initial} />);
    fireEvent.click(screen.getByRole("button", { name: "Điểm danh" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("already"));
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});
