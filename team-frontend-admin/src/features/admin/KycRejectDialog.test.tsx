import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { KycRejectDialog } from "./KycRejectDialog";
import { reviewKycAction } from "./kycActions";
import { KYC_REASON_CODES, KYC_REASON_LABELS } from "./kycReasons";

// Contract under test (implementer creates src/features/admin/KycRejectDialog.tsx):
//   <KycRejectDialog submissionId isOpen onClose />
//   Select labelled "Lý do" with a placeholder and one option per KYC_REASON_CODES
//   (value = code, text = KYC_REASON_LABELS[code]). Button "Xác nhận" calls
//   reviewKycAction(formData) with submissionId, decision "reject", reason.
//   No reason -> "Vui lòng chọn lý do." and no action call. Success -> toast.success
//   (message) and onClose(); failure -> alert with the error, dialog stays open.

vi.mock("./kycActions", () => ({
  reviewKycAction: vi.fn(),
  viewKycDocumentAction: vi.fn(),
}));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const open = () => {
  const onClose = vi.fn();
  render(<KycRejectDialog submissionId="k-1" isOpen onClose={onClose} />);
  return { onClose };
};
const confirm = () => screen.getByRole("button", { name: "Xác nhận" });

describe("KycRejectDialog", () => {
  it("blocks confirm without a reason (KYC queue: Reject without a reason is blocked)", () => {
    open();
    fireEvent.click(confirm());
    expect(screen.getByText("Vui lòng chọn lý do.")).toBeInTheDocument();
    expect(reviewKycAction).not.toHaveBeenCalled();
  });

  it("offers exactly the fixed reason list with Vietnamese labels (KYC queue: Reject with a reason code)", () => {
    open();
    const select = screen.getByLabelText("Lý do") as HTMLSelectElement;
    const withValue = Array.from(select.options).filter((o) => o.value);
    expect(withValue.map((o) => o.value)).toEqual([...KYC_REASON_CODES]);
    for (const o of withValue) {
      expect(o.textContent).toBe(
        KYC_REASON_LABELS[o.value as keyof typeof KYC_REASON_LABELS],
      );
    }
  });

  it("submits reject with the chosen reason code and closes (KYC queue: Reject with a reason code)", async () => {
    vi.mocked(reviewKycAction).mockResolvedValue({
      ok: true,
      message: "Đã từ chối hồ sơ xác minh.",
    });
    const { onClose } = open();
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "document_unreadable" },
    });
    fireEvent.click(confirm());
    await waitFor(() => expect(reviewKycAction).toHaveBeenCalledTimes(1));
    const fd = vi.mocked(reviewKycAction).mock.calls[0][0] as FormData;
    expect(fd.get("submissionId")).toBe("k-1");
    expect(fd.get("decision")).toBe("reject");
    expect(fd.get("reason")).toBe("document_unreadable");
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã từ chối hồ sơ xác minh."),
    );
    expect(onClose).toHaveBeenCalled();
  });

  it("keeps the dialog open and shows the error on failure (KYC queue: Review failure leaves the row unchanged)", async () => {
    vi.mocked(reviewKycAction).mockResolvedValue({
      ok: false,
      error: "Không xử lý được hồ sơ. Vui lòng thử lại sau.",
    });
    const { onClose } = open();
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "other" },
    });
    fireEvent.click(confirm());
    expect(
      await screen.findByText("Không xử lý được hồ sơ. Vui lòng thử lại sau."),
    ).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });
});
