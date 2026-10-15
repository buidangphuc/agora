import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { setupUser } from "@/test/user";
import { ReviewHideButton } from "./ReviewHideButton";
import { hideReviewAction } from "./actions";

vi.mock("./actions", () => ({
  hideReviewAction: vi.fn(),
  createReviewAction: vi.fn(),
  markReviewHelpfulAction: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
}));

function renderButton() {
  render(
    <ToastProvider>
      <ReviewHideButton reviewId="r1" listingId="l1" />
    </ToastProvider>,
  );
}

async function openDialog(user: ReturnType<typeof setupUser>) {
  await user.click(screen.getByTestId("hide-review"));
  return screen.getByRole("dialog", { name: "Ẩn đánh giá" });
}

const submit = () => screen.getByRole("button", { name: /^Ẩn đánh giá$/ });

beforeEach(() => vi.clearAllMocks());

describe("ReviewHideButton", () => {
  it("shows the Ẩn trigger and no dialog until clicked", () => {
    renderButton();
    expect(screen.getByTestId("hide-review")).toHaveTextContent("Ẩn");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("disables submit on empty reason", async () => {
    const user = setupUser();
    renderButton();
    await openDialog(user);
    expect(submit()).toBeDisabled();
    await user.type(screen.getByRole("textbox"), "   ");
    expect(submit()).toBeDisabled();
    await user.type(screen.getByRole("textbox"), "Spam");
    expect(submit()).toBeEnabled();
    expect(hideReviewAction).not.toHaveBeenCalled();
  });

  it("limits the textarea to 500", async () => {
    const user = setupUser();
    renderButton();
    await openDialog(user);
    const box = screen.getByRole("textbox");
    expect(box).toHaveAttribute("maxlength", "500");
    await user.type(box, "Spam");
    expect(screen.getByRole("dialog")).toHaveTextContent("4/500");
  });

  it("sends exactly 500 characters to the action", async () => {
    const user = setupUser();
    vi.mocked(hideReviewAction).mockResolvedValue({ ok: true });
    renderButton();
    await openDialog(user);
    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "a".repeat(500) },
    });
    await user.click(submit());
    expect(hideReviewAction).toHaveBeenCalledWith("r1", "l1", "a".repeat(500));
  });

  it("closes the dialog and toasts on success", async () => {
    const user = setupUser();
    vi.mocked(hideReviewAction).mockResolvedValue({ ok: true });
    renderButton();
    await openDialog(user);
    await user.type(screen.getByRole("textbox"), "Spam quảng cáo");
    await user.click(submit());
    expect(hideReviewAction).toHaveBeenCalledTimes(1);
    expect(hideReviewAction).toHaveBeenCalledWith("r1", "l1", "Spam quảng cáo");
    expect(await screen.findByText("Đã ẩn đánh giá.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("keeps the dialog open on error", async () => {
    const user = setupUser();
    vi.mocked(hideReviewAction).mockResolvedValue({
      ok: false,
      error: "Chưa ẩn được đánh giá. Vui lòng thử lại sau.",
    });
    renderButton();
    await openDialog(user);
    await user.type(screen.getByRole("textbox"), "Spam");
    await user.click(submit());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Chưa ẩn được đánh giá. Vui lòng thử lại sau.",
    );
    expect(
      screen.getByRole("dialog", { name: "Ẩn đánh giá" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toHaveValue("Spam");
    expect(screen.queryByText("Đã ẩn đánh giá.")).toBeNull();
  });

  it("sends one request while pending", async () => {
    const user = setupUser();
    let resolve: (v: { ok: true }) => void = () => {};
    vi.mocked(hideReviewAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    renderButton();
    await openDialog(user);
    await user.type(screen.getByRole("textbox"), "Spam");
    await user.click(submit());
    await user.click(submit());
    expect(hideReviewAction).toHaveBeenCalledTimes(1);
    expect(submit()).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("textbox")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Hủy" })).toBeDisabled();

    await act(async () => {
      resolve({ ok: true });
    });
    expect(await screen.findByText("Đã ẩn đánh giá.")).toBeInTheDocument();
    expect(hideReviewAction).toHaveBeenCalledTimes(1);
  });
});
