import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ModerateDialog } from "./ModerateDialog";
import { moderateListingAction } from "./moderationActions";

// Contract under test (implementer creates src/features/admin/ModerateDialog.tsx):
//   <ModerateDialog listingId decision? isOpen onClose />
//   decision?: "reject" | "ban". When omitted the dialog offers a select labelled
//   "Quyết định" with options "Từ chối" (reject) and "Khóa" (ban).
//   Select labelled "Lý do" (placeholder, no seller_enforcement): prohibited_item
//   "Hàng cấm", counterfeit "Hàng giả", misleading, inappropriate, duplicate, other.
//   Textarea labelled /Ghi chú/ (maxLength 500, counter "n/500").
//   Confirm button "Xác nhận" calls moderateListingAction(listingId, decision, reason, note);
//   success -> toast.success(message) and onClose(); failure -> Alert with the
//   error, dialog stays open. A pending call disables the confirm button.

vi.mock("./moderationActions", () => ({ moderateListingAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const open = (props: Record<string, unknown> = {}) => {
  const onClose = vi.fn();
  render(
    <ModerateDialog
      listingId="l-1"
      decision="reject"
      isOpen
      onClose={onClose}
      {...props}
    />,
  );
  return { onClose };
};

const confirm = () => screen.getByRole("button", { name: "Xác nhận" });

describe("ModerateDialog", () => {
  it("blocks submit without a reason code (Moderation UI: Reject needs a reason code)", () => {
    open();
    fireEvent.click(confirm());
    expect(screen.getByText("Vui lòng chọn lý do.")).toBeInTheDocument();
    expect(moderateListingAction).not.toHaveBeenCalled();
  });

  it("does not offer seller_enforcement (Moderation UI: Reject needs a reason code)", () => {
    open();
    const select = screen.getByLabelText("Lý do") as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    expect(values).not.toContain("seller_enforcement");
    for (const v of [
      "prohibited_item",
      "counterfeit",
      "misleading",
      "inappropriate",
      "duplicate",
      "other",
    ]) {
      expect(values).toContain(v);
    }
  });

  it("caps the note at 500 characters (Moderation UI: Moderation note length boundary)", () => {
    open();
    const note = screen.getByLabelText(/Ghi chú/) as HTMLTextAreaElement;
    expect(note).toHaveAttribute("maxlength", "500");
    fireEvent.change(note, { target: { value: "a".repeat(501) } });
    expect(note.value).toHaveLength(500);
    expect(screen.getByText("500/500")).toBeInTheDocument();
  });

  it("sends reason and note then toasts and closes (Moderation UI: Reject sends reason and note)", async () => {
    vi.mocked(moderateListingAction).mockResolvedValue({
      ok: true,
      message: "Đã từ chối tin đăng.",
    });
    const { onClose } = open();
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "counterfeit" },
    });
    fireEvent.change(screen.getByLabelText(/Ghi chú/), {
      target: { value: "Ảnh sản phẩm là hàng nhái" },
    });
    fireEvent.click(confirm());
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(moderateListingAction).toHaveBeenCalledTimes(1);
    expect(moderateListingAction).toHaveBeenCalledWith(
      "l-1",
      "reject",
      "counterfeit",
      "Ảnh sản phẩm là hàng nhái",
    );
    expect(toastSuccess).toHaveBeenCalledWith("Đã từ chối tin đăng.");
  });

  it("ban decision sends ban (Moderation UI: Ban removes the listing from the queue)", async () => {
    vi.mocked(moderateListingAction).mockResolvedValue({
      ok: true,
      message: "Đã khóa tin đăng.",
    });
    open({ decision: "ban" });
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "prohibited_item" },
    });
    fireEvent.click(confirm());
    await waitFor(() => expect(moderateListingAction).toHaveBeenCalledTimes(1));
    expect(moderateListingAction).toHaveBeenCalledWith(
      "l-1",
      "ban",
      "prohibited_item",
      "",
    );
  });

  it("pending state blocks a double submit (Moderation UI: Moderation double submit is blocked)", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(moderateListingAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    open();
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "other" },
    });
    fireEvent.click(confirm());
    fireEvent.click(confirm());
    await waitFor(() => expect(confirm()).toBeDisabled());
    fireEvent.click(confirm());
    expect(moderateListingAction).toHaveBeenCalledTimes(1);
    resolve({ ok: true, message: "Đã từ chối tin đăng." });
  });

  it("shows the failure and keeps the dialog open (Moderation UI: Audit unavailable leaves the row)", async () => {
    const msg =
      "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau.";
    vi.mocked(moderateListingAction).mockResolvedValue({
      ok: false,
      error: msg,
    });
    const { onClose } = open();
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "other" },
    });
    fireEvent.click(confirm());
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(msg),
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(toastSuccess).not.toHaveBeenCalled();
  });

  it("offers a decision select when none is fixed (Moderation UI: Removed listing is labelled)", async () => {
    vi.mocked(moderateListingAction).mockResolvedValue({
      ok: true,
      message: "Đã khóa tin đăng.",
    });
    open({ decision: undefined });
    fireEvent.change(screen.getByLabelText("Quyết định"), {
      target: { value: "ban" },
    });
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "other" },
    });
    fireEvent.click(confirm());
    await waitFor(() => expect(moderateListingAction).toHaveBeenCalledTimes(1));
    expect(moderateListingAction).toHaveBeenCalledWith(
      "l-1",
      "ban",
      "other",
      "",
    );
  });

  it("renders nothing when closed (Moderation UI: Reject needs a reason code)", () => {
    open({ isOpen: false });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
