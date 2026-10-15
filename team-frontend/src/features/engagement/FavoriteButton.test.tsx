import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce, addFavoriteAction, removeFavoriteAction, toast } =
  vi.hoisted(() => ({
    trackEcommerce: vi.fn(),
    addFavoriteAction: vi.fn(),
    removeFavoriteAction: vi.fn(),
    toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("./actions", () => ({ addFavoriteAction, removeFavoriteAction }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { FavoriteButton } from "./FavoriteButton";

beforeEach(() => vi.clearAllMocks());

describe("FavoriteButton loop events", () => {
  it("tracks add_to_wishlist after success", async () => {
    addFavoriteAction.mockResolvedValue(undefined);
    render(<FavoriteButton id="L1" initial={false} />);
    fireEvent.click(screen.getByRole("button", { name: "Yêu thích" }));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce).toHaveBeenCalledWith("add_to_wishlist", {
      items: [{ itemId: "L1" }],
    });
  });

  it("tracks nothing on failure or removal: add rejects and the heart rolls back", async () => {
    addFavoriteAction.mockRejectedValue(new Error("boom"));
    render(<FavoriteButton id="L1" initial={false} />);
    fireEvent.click(screen.getByRole("button", { name: "Yêu thích" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: "Yêu thích" })).toBeTruthy();
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("tracks nothing on failure or removal: un-favoriting", async () => {
    removeFavoriteAction.mockResolvedValue(undefined);
    render(<FavoriteButton id="L1" initial={true} />);
    fireEvent.click(screen.getByRole("button", { name: "Bỏ thích" }));
    await waitFor(() => expect(removeFavoriteAction).toHaveBeenCalledWith("L1"));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Yêu thích" })).toBeTruthy(),
    );
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});
