import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce, createShareLinkAction, toast } = vi.hoisted(() => ({
  trackEcommerce: vi.fn(),
  createShareLinkAction: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("./actions", () => ({ createShareLinkAction }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { ShareButton } from "./ShareButton";

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});

describe("ShareButton loop events", () => {
  it("tracks create and each copy", async () => {
    createShareLinkAction.mockResolvedValue({ ok: true, shortCode: "abc" });
    render(<ShareButton id="L1" />);
    fireEvent.click(screen.getByRole("button", { name: /Chia sẻ/ }));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    const copy = await screen.findByRole("button", {
      name: /Sao chép liên kết/,
    });
    fireEvent.click(copy);
    fireEvent.click(copy);
    expect(trackEcommerce).toHaveBeenCalledTimes(3);
    for (const call of trackEcommerce.mock.calls)
      expect(call).toEqual([
        "share",
        {
          items: [{ itemId: "L1" }],
          properties: { method: "copy_link", content_type: "listing" },
        },
      ]);
    expect(JSON.stringify(trackEcommerce.mock.calls)).not.toContain("abc");
  });

  it("tracks nothing when creation fails: ok false", async () => {
    createShareLinkAction.mockResolvedValue({ ok: false, message: "nope" });
    render(<ShareButton id="L1" />);
    fireEvent.click(screen.getByRole("button", { name: /Chia sẻ/ }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("nope"));
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("tracks nothing when creation fails: ok true without shortCode", async () => {
    createShareLinkAction.mockResolvedValue({ ok: true });
    render(<ShareButton id="L1" />);
    fireEvent.click(screen.getByRole("button", { name: /Chia sẻ/ }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});
