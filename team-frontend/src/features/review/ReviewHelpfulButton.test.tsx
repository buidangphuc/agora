import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { setupUser } from "@/test/user";
import { ReviewHelpfulButton } from "./ReviewHelpfulButton";
import { markReviewHelpfulAction } from "./actions";

vi.mock("./actions", () => ({
  markReviewHelpfulAction: vi.fn(),
  createReviewAction: vi.fn(),
}));

function renderButton() {
  render(
    <ToastProvider>
      <ReviewHelpfulButton reviewId="r1" listingId="L" initialCount={3} />
    </ToastProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("ReviewHelpfulButton", () => {
  it("bumps the count at once, takes the authoritative count and cannot be voted again", async () => {
    const user = setupUser();
    let resolve: (v: { ok: true; data: { helpfulCount: number } }) => void =
      () => {};
    vi.mocked(markReviewHelpfulAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    renderButton();
    const btn = screen.getByTestId("review-helpful");
    expect(btn).toHaveTextContent("Hữu ích (3)");

    await user.click(btn);
    expect(markReviewHelpfulAction).toHaveBeenCalledWith("r1", "L");
    expect(btn).toHaveTextContent("Hữu ích (4)");
    expect(btn).toBeDisabled();

    await act(async () => {
      resolve({ ok: true, data: { helpfulCount: 9 } });
    });
    expect(btn).toHaveTextContent("Hữu ích (9)");
    expect(btn).toBeDisabled();
  });

  it("reverts the count, shows an error toast and allows another try on failure", async () => {
    const user = setupUser();
    vi.mocked(markReviewHelpfulAction).mockResolvedValue({
      ok: false,
      error: "Bạn đã bình chọn rồi",
    });
    renderButton();
    const btn = screen.getByTestId("review-helpful");
    await user.click(btn);
    expect(await screen.findByText("Bạn đã bình chọn rồi")).toBeInTheDocument();
    expect(btn).toHaveTextContent("Hữu ích (3)");
    expect(btn).toBeEnabled();
  });
});
