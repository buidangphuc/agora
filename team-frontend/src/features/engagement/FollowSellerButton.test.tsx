import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce, followSellerAction, unfollowSellerAction, toast } =
  vi.hoisted(() => ({
    trackEcommerce: vi.fn(),
    followSellerAction: vi.fn(),
    unfollowSellerAction: vi.fn(),
    toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("./actions", () => ({ followSellerAction, unfollowSellerAction }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { FollowSellerButton } from "./FollowSellerButton";

beforeEach(() => vi.clearAllMocks());

describe("FollowSellerButton loop events", () => {
  it("tracks follow_seller on success", async () => {
    followSellerAction.mockResolvedValue({ ok: true });
    render(<FollowSellerButton sellerId="S1" initialFollowing={false} />);
    fireEvent.click(screen.getByRole("button", { name: "+ Theo dõi" }));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce).toHaveBeenCalledWith("follow_seller", {
      properties: { seller_id: "S1" },
    });
  });

  it("tracks nothing on unfollow or failure: unfollow", async () => {
    unfollowSellerAction.mockResolvedValue({ ok: true });
    render(<FollowSellerButton sellerId="S1" initialFollowing={true} />);
    fireEvent.click(screen.getByRole("button", { name: "✓ Đang theo dõi" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("tracks nothing on unfollow or failure: failed follow rolls back", async () => {
    followSellerAction.mockResolvedValue({ ok: false, message: "no" });
    render(<FollowSellerButton sellerId="S1" initialFollowing={false} />);
    fireEvent.click(screen.getByRole("button", { name: "+ Theo dõi" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: "+ Theo dõi" })).toBeTruthy();
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});
