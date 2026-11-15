import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { KycQueueView } from "./KycQueueView";
import { reviewKycAction, viewKycDocumentAction } from "./kycActions";

// Contract under test (implementer creates src/features/admin/KycQueueView.tsx):
//   <KycQueueView items nextCursor status />
//   items: KycSubmissionView[] (id, userId, docType, status, documentCount,
//   reasonCode, createdAt ISO, reviewedAt) rendered in the given order as table
//   rows (role "row" after the header). createdAt shown as YYYY-MM-DD HH:mm:ss
//   UTC+07:00. status: current filter "PENDING"|"VERIFIED"|"REJECTED".
//   Empty list -> "Không có hồ sơ nào đang chờ duyệt.". nextCursor "" -> no
//   "Trang sau" link, else link whose href has tab=kyc and cursor=<c>.
//   Per row: buttons "Xem tài liệu N" for N=1..documentCount; click calls
//   viewKycDocumentAction(formData{submissionId,index=N-1}); success reveals link
//   "Mở tài liệu" (target _blank, rel "noopener noreferrer") and the expiry;
//   failure shows the error in an alert. "Duyệt" (reviewKycAction approve) and
//   "Từ chối" (opens KycRejectDialog) only on PENDING rows.

vi.mock("./kycActions", () => ({
  reviewKycAction: vi.fn(),
  viewKycDocumentAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const row = (over: Record<string, unknown> = {}) => ({
  id: "k-1",
  userId: "u-seller-1",
  docType: "cccd",
  status: "PENDING" as const,
  documentCount: 2,
  reasonCode: "",
  createdAt: "2026-10-05T17:00:00.000Z",
  reviewedAt: "",
  ...over,
});

const view = (items: ReturnType<typeof row>[], extra = {}) =>
  render(
    <KycQueueView items={items} nextCursor="" status="PENDING" {...extra} />,
  );

describe("KycQueueView", () => {
  it("renders rows in the given order with user id, count and UTC+07:00 time (KYC queue: Pending submissions are listed oldest first)", () => {
    view([
      row({ id: "k-1", userId: "u-a", createdAt: "2026-10-05T17:00:00.000Z" }),
      row({ id: "k-2", userId: "u-b", createdAt: "2026-10-06T01:00:00.000Z" }),
      row({ id: "k-3", userId: "u-c", createdAt: "2026-10-07T01:00:00.000Z" }),
    ]);
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows).toHaveLength(3);
    expect(rows[0].textContent).toContain("u-a");
    expect(rows[1].textContent).toContain("u-b");
    expect(rows[2].textContent).toContain("u-c");
    expect(rows[0].textContent).toContain("2026-10-06 00:00:00");
    expect(rows[1].textContent).toContain("2026-10-06 08:00:00");
    expect(rows[0].textContent).toContain("k-1");
  });

  it("shows a Trang sau link with the cursor only when there is a next page (KYC queue: Page boundary at 20 rows)", () => {
    const items = Array.from({ length: 20 }, (_, i) =>
      row({ id: `k-${i}`, userId: `u-${i}` }),
    );
    const { unmount } = view(items, { nextCursor: "cur-20" });
    expect(screen.getAllByRole("row").slice(1)).toHaveLength(20);
    const next = screen.getByRole("link", { name: "Trang sau" });
    expect(next.getAttribute("href")).toContain("tab=kyc");
    expect(next.getAttribute("href")).toContain("cursor=cur-20");
    unmount();
    view([row()], { nextCursor: "" });
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("shows the empty message and no rows (KYC queue: Empty queue)", () => {
    view([]);
    expect(
      screen.getByText("Không có hồ sơ nào đang chờ duyệt."),
    ).toBeInTheDocument();
    expect(screen.queryAllByRole("row").length).toBeLessThanOrEqual(1);
    expect(screen.queryByRole("button", { name: /Xem tài liệu/ })).toBeNull();
  });

  it("shows decided rows with status, reason code and no action buttons (KYC queue: Decided submissions are reachable through the filter)", () => {
    view(
      [
        row({
          status: "REJECTED",
          reasonCode: "document_unreadable",
          reviewedAt: "2026-10-06T00:00:00.000Z",
        }),
      ],
      { status: "REJECTED" },
    );
    expect(screen.getByText("Đã từ chối")).toBeInTheDocument();
    expect(screen.getByText("document_unreadable")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Duyệt" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Từ chối" })).toBeNull();
  });

  it("shows exactly documentCount view buttons (KYC queue: Document button count matches the document count)", () => {
    view([row({ documentCount: 3 })]);
    const btns = screen.getAllByRole("button", { name: /^Xem tài liệu \d+$/ });
    expect(btns.map((b) => b.textContent)).toEqual([
      "Xem tài liệu 1",
      "Xem tài liệu 2",
      "Xem tài liệu 3",
    ]);
    expect(screen.queryByRole("button", { name: "Xem tài liệu 4" })).toBeNull();
  });

  it("contains no link or URL before any click (KYC queue: No document URL is present before the click)", () => {
    const { container } = view([row()]);
    expect(screen.queryByRole("link", { name: "Mở tài liệu" })).toBeNull();
    expect(container.innerHTML).not.toMatch(/https?:\/\//);
    expect(viewKycDocumentAction).not.toHaveBeenCalled();
  });

  it("reveals the link with expiry after a successful view and calls the action once with the zero-based index (KYC queue: Viewing a document reveals a 5-minute link and writes the audit entry)", async () => {
    vi.mocked(viewKycDocumentAction).mockResolvedValue({
      ok: true,
      url: "https://files.example/kyc/doc-2?sig=abc",
      expiresAt: "2026-10-05T17:05:00.000Z",
    });
    view([row({ documentCount: 2 })]);
    fireEvent.click(screen.getByRole("button", { name: "Xem tài liệu 2" }));
    const link = await screen.findByRole("link", { name: "Mở tài liệu" });
    expect(link).toHaveAttribute(
      "href",
      "https://files.example/kyc/doc-2?sig=abc",
    );
    expect(link).toHaveAttribute("target", "_blank");
    expect(link.getAttribute("rel")).toContain("noopener");
    expect(link.getAttribute("rel")).toContain("noreferrer");
    expect(document.body.textContent).toContain("2026-10-06 00:05:00");
    expect(viewKycDocumentAction).toHaveBeenCalledTimes(1);
    const fd = vi.mocked(viewKycDocumentAction).mock.calls[0][0] as FormData;
    expect(fd.get("submissionId")).toBe("k-1");
    expect(fd.get("index")).toBe("1");
  });

  it("shows the refusal and no link when the view fails closed (KYC queue: Audit store down fails closed)", async () => {
    vi.mocked(viewKycDocumentAction).mockResolvedValue({
      ok: false,
      error:
        "Không ghi được nhật ký truy cập nên chưa mở được tài liệu. Vui lòng thử lại sau.",
    });
    view([row()]);
    fireEvent.click(screen.getByRole("button", { name: "Xem tài liệu 1" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Không ghi được nhật ký truy cập nên chưa mở được tài liệu.",
    );
    expect(screen.queryByRole("link", { name: "Mở tài liệu" })).toBeNull();
  });

  it("offers Duyệt and Từ chối on pending rows and approves with the submission id (KYC queue: Approve removes the row and verifies the seller)", async () => {
    vi.mocked(reviewKycAction).mockResolvedValue({
      ok: true,
      message: "Đã duyệt hồ sơ xác minh.",
    });
    view([row()]);
    expect(screen.getByRole("button", { name: "Từ chối" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Duyệt" }));
    await waitFor(() => expect(reviewKycAction).toHaveBeenCalledTimes(1));
    const fd = vi.mocked(reviewKycAction).mock.calls[0][0] as FormData;
    expect(fd.get("submissionId")).toBe("k-1");
    expect(fd.get("decision")).toBe("approve");
  });

  it("opens the reject dialog that blocks an empty reason (KYC queue: Reject without a reason is blocked)", () => {
    view([row()]);
    fireEvent.click(screen.getByRole("button", { name: "Từ chối" }));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Xác nhận" }));
    expect(screen.getByText("Vui lòng chọn lý do.")).toBeInTheDocument();
    expect(reviewKycAction).not.toHaveBeenCalled();
  });
});
