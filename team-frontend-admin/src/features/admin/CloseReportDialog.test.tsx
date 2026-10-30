import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CloseReportDialog } from "./CloseReportDialog";
import { closeReportAction } from "./reportActions";

// Contract under test (implementer creates src/features/admin/CloseReportDialog.tsx):
//   <CloseReportDialog reportId isOpen onClose />
//   Select labelled "Kết quả": "actioned" ("Đã xử lý") / "dismissed" ("Bác bỏ").
//   Textarea labelled /Ghi chú xử lý/ (maxLength 500).
//   Confirm button "Xác nhận" calls closeReportAction(reportId, outcome, note);
//   empty/whitespace note -> "Vui lòng nhập ghi chú xử lý." and no call;
//   success -> toast.success(message) + onClose(); failure -> Alert, stays open;
//   pending disables the confirm button.

vi.mock("./reportActions", () => ({ closeReportAction: vi.fn() }));
const toastSuccess = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: vi.fn() }),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const open = () => {
  const onClose = vi.fn();
  render(<CloseReportDialog reportId="r-1" isOpen onClose={onClose} />);
  return { onClose };
};
const confirm = () => screen.getByRole("button", { name: "Xác nhận" });

describe("CloseReportDialog", () => {
  it("blocks an empty note (Moderation UI: Close needs a note)", () => {
    open();
    fireEvent.change(screen.getByLabelText("Kết quả"), {
      target: { value: "actioned" },
    });
    fireEvent.click(confirm());
    expect(
      screen.getByText("Vui lòng nhập ghi chú xử lý."),
    ).toBeInTheDocument();
    expect(closeReportAction).not.toHaveBeenCalled();
  });

  it("blocks a whitespace-only note (Moderation UI: Close needs a note)", () => {
    open();
    fireEvent.change(screen.getByLabelText("Kết quả"), {
      target: { value: "dismissed" },
    });
    fireEvent.change(screen.getByLabelText(/Ghi chú xử lý/), {
      target: { value: "    " },
    });
    fireEvent.click(confirm());
    expect(
      screen.getByText("Vui lòng nhập ghi chú xử lý."),
    ).toBeInTheDocument();
    expect(closeReportAction).not.toHaveBeenCalled();
  });

  it("sends outcome and note then toasts and closes (Moderation UI: Close a report as actioned)", async () => {
    vi.mocked(closeReportAction).mockResolvedValue({
      ok: true,
      message: "Đã đóng báo cáo.",
    });
    const { onClose } = open();
    fireEvent.change(screen.getByLabelText("Kết quả"), {
      target: { value: "actioned" },
    });
    fireEvent.change(screen.getByLabelText(/Ghi chú xử lý/), {
      target: { value: "Đã khóa tin đăng" },
    });
    fireEvent.click(confirm());
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(closeReportAction).toHaveBeenCalledTimes(1);
    expect(closeReportAction).toHaveBeenCalledWith(
      "r-1",
      "actioned",
      "Đã khóa tin đăng",
    );
    expect(toastSuccess).toHaveBeenCalledWith("Đã đóng báo cáo.");
  });

  it("dismiss outcome is sent as dismissed (Moderation UI: Dismiss a report)", async () => {
    vi.mocked(closeReportAction).mockResolvedValue({
      ok: true,
      message: "Đã đóng báo cáo.",
    });
    open();
    fireEvent.change(screen.getByLabelText("Kết quả"), {
      target: { value: "dismissed" },
    });
    fireEvent.change(screen.getByLabelText(/Ghi chú xử lý/), {
      target: { value: "Không vi phạm" },
    });
    fireEvent.click(confirm());
    await waitFor(() => expect(closeReportAction).toHaveBeenCalledTimes(1));
    expect(closeReportAction).toHaveBeenCalledWith(
      "r-1",
      "dismissed",
      "Không vi phạm",
    );
  });

  it("pending state blocks a double submit (Moderation UI: Close needs a note)", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(closeReportAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    open();
    fireEvent.change(screen.getByLabelText("Kết quả"), {
      target: { value: "actioned" },
    });
    fireEvent.change(screen.getByLabelText(/Ghi chú xử lý/), {
      target: { value: "ok" },
    });
    fireEvent.click(confirm());
    fireEvent.click(confirm());
    await waitFor(() => expect(confirm()).toBeDisabled());
    fireEvent.click(confirm());
    expect(closeReportAction).toHaveBeenCalledTimes(1);
    resolve({ ok: true, message: "Đã đóng báo cáo." });
  });

  it("shows a failure and stays open (Moderation UI: Close fails closed when audit is down)", async () => {
    const msg =
      "Chưa ghi được nhật ký kiểm toán nên thao tác chưa được thực hiện. Vui lòng thử lại sau.";
    vi.mocked(closeReportAction).mockResolvedValue({ ok: false, error: msg });
    const { onClose } = open();
    fireEvent.change(screen.getByLabelText("Kết quả"), {
      target: { value: "actioned" },
    });
    fireEvent.change(screen.getByLabelText(/Ghi chú xử lý/), {
      target: { value: "ok" },
    });
    fireEvent.click(confirm());
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(msg),
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(toastSuccess).not.toHaveBeenCalled();
  });
});
