import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReportListingButton } from "./ReportListingButton";
import { reportListingAction } from "./reportActions";

vi.mock("./reportActions", () => ({ reportListingAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

function openDialog() {
  fireEvent.click(screen.getByRole("button", { name: "Báo cáo tin đăng" }));
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ReportListingButton", () => {
  it("renders for a signed-in non-owner", () => {
    // Moderation UI: Signed-in buyer sees the report control
    render(<ReportListingButton listingId="L" loggedIn />);
    expect(
      screen.getByRole("button", { name: "Báo cáo tin đăng" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("renders a login link returning to the listing for a guest", () => {
    // Moderation UI: Guest is sent to login and back
    render(<ReportListingButton listingId="L" loggedIn={false} />);
    expect(
      screen.getByRole("link", { name: "Báo cáo tin đăng" }),
    ).toHaveAttribute("href", "/login?returnUrl=%2Flisting%2FL");
  });

  it("blocks submit without a reason", async () => {
    // Moderation UI: Reason is required
    render(<ReportListingButton listingId="L" loggedIn />);
    openDialog();
    fireEvent.click(screen.getByRole("button", { name: "Gửi báo cáo" }));
    expect(await screen.findByText("Vui lòng chọn lý do.")).toBeVisible();
    expect(reportListingAction).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("pending state blocks a double submit", async () => {
    // Moderation UI: Report double submit is blocked
    let resolve: (v: { ok: true; message: string }) => void = () => {};
    vi.mocked(reportListingAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    render(<ReportListingButton listingId="L" loggedIn />);
    openDialog();
    fireEvent.change(screen.getByLabelText("Lý do"), {
      target: { value: "scam" },
    });
    const submit = screen.getByRole("button", { name: "Gửi báo cáo" });
    fireEvent.click(submit);
    fireEvent.click(submit);
    await waitFor(() => expect(submit).toBeDisabled());
    fireEvent.click(submit);
    expect(reportListingAction).toHaveBeenCalledTimes(1);
    expect(reportListingAction).toHaveBeenCalledWith("L", "scam", "");

    resolve({ ok: true, message: "Đã gửi báo cáo. Cảm ơn bạn." });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(toastSuccess).toHaveBeenCalledWith("Đã gửi báo cáo. Cảm ơn bạn.");
  });

  it("note counter reaches 500 and the textarea caps at 500", () => {
    // Moderation UI: Report note length boundary (task 1.7 note counter 500)
    render(<ReportListingButton listingId="L" loggedIn />);
    openDialog();
    const note = screen.getByLabelText(/Chi tiết/) as HTMLTextAreaElement;
    expect(note).toHaveAttribute("maxlength", "500");
    fireEvent.change(note, { target: { value: "a".repeat(500) } });
    expect(screen.getByText("500/500")).toBeInTheDocument();
  });
});
