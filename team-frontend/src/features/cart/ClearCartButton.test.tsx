import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import { ClearCartButton } from "./ClearCartButton";
import { clearCartAction } from "./actions";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("./actions", () => ({ clearCartAction: vi.fn() }));

beforeEach(() => vi.clearAllMocks());
afterEach(() => vi.unstubAllGlobals());

describe("ClearCartButton", () => {
  it("keeps the 'Xóa tất cả' label, is loading while pending, then toasts", async () => {
    const user = setupUser();
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
    let resolve!: (v: { ok: true }) => void;
    const pending = new Promise<{ ok: true }>((r) => {
      resolve = r;
    });
    vi.mocked(clearCartAction).mockReturnValue(pending);
    render(<ClearCartButton />);

    await user.click(screen.getByRole("button", { name: "Xóa tất cả" }));
    const btn = screen.getByRole("button", { name: "Xóa tất cả" });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute("aria-busy", "true");

    await act(async () => {
      resolve({ ok: true });
      await pending;
    });
    expect(toast.info).toHaveBeenCalledWith("Đã làm trống giỏ hàng.");
    expect(screen.getByRole("button", { name: "Xóa tất cả" })).toBeEnabled();
  });

  it("does nothing when the confirmation is declined", async () => {
    const user = setupUser();
    vi.stubGlobal(
      "confirm",
      vi.fn(() => false),
    );
    render(<ClearCartButton />);
    await user.click(screen.getByRole("button", { name: "Xóa tất cả" }));
    expect(clearCartAction).not.toHaveBeenCalled();
  });

  it("shows an error toast on failure", async () => {
    const user = setupUser();
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
    vi.mocked(clearCartAction).mockResolvedValue({ ok: false, error: "lỗi" });
    render(<ClearCartButton />);
    await user.click(screen.getByRole("button", { name: "Xóa tất cả" }));
    expect(toast.error).toHaveBeenCalledWith("lỗi");
  });
});
