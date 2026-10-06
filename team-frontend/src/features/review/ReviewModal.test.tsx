import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { setupUser } from "@/test/user";
import { ReviewModal } from "./ReviewModal";
import { createReviewAction } from "./actions";

vi.mock("./actions", () => ({
  createReviewAction: vi.fn(),
  markReviewHelpfulAction: vi.fn(),
}));

function renderModal(
  props: Partial<React.ComponentProps<typeof ReviewModal>> = {},
) {
  const onClose = vi.fn();
  const onSuccess = vi.fn();
  render(
    <ToastProvider>
      <ReviewModal
        listingId="l1"
        productTitle="Áo thun"
        onClose={onClose}
        onSuccess={onSuccess}
        {...props}
      />
    </ToastProvider>,
  );
  return { onClose, onSuccess };
}

beforeEach(() => vi.clearAllMocks());

describe("ReviewModal", () => {
  it("goes pending -> success: loading submit, disabled controls, then toast, onSuccess and close", async () => {
    const user = setupUser();
    let resolve: (v: { ok: true }) => void = () => {};
    vi.mocked(createReviewAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    const { onClose, onSuccess } = renderModal({ orderId: "o1" });

    await user.click(screen.getByRole("radio", { name: "4 sao" }));
    await user.type(screen.getByLabelText("Nhận xét chi tiết"), "  Tốt lắm  ");
    await user.click(screen.getByRole("button", { name: "Hoàn thành" }));

    expect(createReviewAction).toHaveBeenCalledWith(
      "l1",
      4,
      "Tốt lắm",
      "o1",
      [],
    );
    const submit = screen.getByRole("button", { name: "Hoàn thành" });
    expect(submit).toHaveAttribute("aria-busy", "true");
    expect(screen.getByLabelText("Nhận xét chi tiết")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Hủy" })).toBeDisabled();
    expect(onClose).not.toHaveBeenCalled();

    await act(async () => {
      resolve({ ok: true });
    });

    expect(
      await screen.findByText("Đánh giá sản phẩm thành công!"),
    ).toBeInTheDocument();
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("keeps the modal open with an inline Alert when the action fails", async () => {
    const user = setupUser();
    vi.mocked(createReviewAction).mockResolvedValue({
      ok: false,
      error: "Bạn đã đánh giá sản phẩm này",
    });
    const { onClose } = renderModal();
    await user.type(screen.getByLabelText("Nhận xét chi tiết"), "ok");
    await user.click(screen.getByRole("button", { name: "Hoàn thành" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Bạn đã đánh giá sản phẩm này",
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Hoàn thành" })).toBeEnabled();
    expect(screen.getByLabelText("Nhận xét chi tiết")).toBeEnabled();
  });

  it("blocks an empty comment without calling the action", async () => {
    const user = setupUser();
    renderModal();
    await user.click(screen.getByRole("button", { name: "Hoàn thành" }));
    expect(createReviewAction).not.toHaveBeenCalled();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Vui lòng nhập nội dung đánh giá.",
    );
  });

  it("shows a generic Alert when the action throws", async () => {
    const user = setupUser();
    vi.mocked(createReviewAction).mockRejectedValue(new Error("boom"));
    renderModal();
    await user.type(screen.getByLabelText("Nhận xét chi tiết"), "ok");
    await user.click(screen.getByRole("button", { name: "Hoàn thành" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Có lỗi xảy ra khi gửi đánh giá.",
    );
  });
});
