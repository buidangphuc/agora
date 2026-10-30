import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ReportsView } from "./ReportsView";
import { moderateListingAction } from "./moderationActions";

// Contract under test (implementer creates src/features/admin/ReportsView.tsx):
//   <ReportsView reports nextCursor status />
//   reports: { id, listingId, reasonCode, note, reporterId,
//              status: "open"|"actioned"|"dismissed", createdAt /*ISO*/,
//              resolutionNote }[]
//   status: "OPEN" | "ACTIONED" | "DISMISSED" (current filter, from parseCasesParams)
//   Filter links: "Đang mở" / "Đã xử lý" / "Đã bác bỏ" ->
//     /admin/cases?tab=reports&status=OPEN|ACTIONED|DISMISSED
//   Next link: /admin/cases?tab=reports&status=<status>&cursor=<c>
//   Row buttons: "Xử lý tin đăng" (ModerateDialog with no fixed decision) and
//   "Đóng báo cáo" (CloseReportDialog). When moderateListingAction answers
//   { ok:false, error:"Tin đăng không còn tồn tại." } the row shows
//   "Tin đăng đã bị gỡ" and "Đóng báo cáo" stays available.

vi.mock("./moderationActions", () => ({ moderateListingAction: vi.fn() }));
vi.mock("./reportActions", () => ({ closeReportAction: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const report = (over: Record<string, unknown> = {}) => ({
  id: "r-1",
  listingId: "l-1",
  reasonCode: "scam",
  note: "Giá quá rẻ",
  reporterId: "u-9",
  status: "open" as const,
  createdAt: "2026-10-05T17:00:00.000Z",
  resolutionNote: "",
  ...over,
});

const base = { reports: [report()], nextCursor: "", status: "OPEN" as const };

describe("ReportsView", () => {
  it("renders one report row (Moderation UI: Report row shows reporter to the admin)", () => {
    render(<ReportsView {...base} />);
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    const text = rows[1].textContent ?? "";
    expect(text).toContain("l-1");
    expect(text).toContain("Lừa đảo");
    expect(text).toContain("Giá quá rẻ");
    expect(text).toContain("u-9");
    expect(text).toContain("2026-10-06 00:00:00");
    for (const h of [
      "Tin đăng",
      "Lý do",
      "Ghi chú",
      "Người báo cáo",
      "Báo cáo lúc",
      "Trạng thái",
    ]) {
      expect(screen.getByRole("columnheader", { name: h })).toBeInTheDocument();
    }
  });

  it("escapes report notes (Moderation UI: Report note markup is shown as text)", () => {
    const { container } = render(
      <ReportsView
        {...base}
        reports={[report({ note: "<img src=x onerror=alert(1)>" })]}
      />,
    );
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
    expect(container.querySelectorAll("img")).toHaveLength(0);
  });

  it("shows the empty state (Moderation UI: Empty report list)", () => {
    render(<ReportsView {...base} reports={[]} />);
    expect(screen.getByText("Không có báo cáo nào")).toBeInTheDocument();
    expect(screen.queryAllByRole("row").length).toBeLessThanOrEqual(1);
  });

  it("status filter links keep the tab (Moderation UI: Status filter reloads the list)", () => {
    render(<ReportsView {...base} />);
    expect(screen.getByRole("link", { name: "Đang mở" })).toHaveAttribute(
      "href",
      "/admin/cases?tab=reports&status=OPEN",
    );
    expect(screen.getByRole("link", { name: "Đã xử lý" })).toHaveAttribute(
      "href",
      "/admin/cases?tab=reports&status=ACTIONED",
    );
    expect(screen.getByRole("link", { name: "Đã bác bỏ" })).toHaveAttribute(
      "href",
      "/admin/cases?tab=reports&status=DISMISSED",
    );
  });

  it("next link keeps tab and status, hidden on the last page (Moderation UI: Status filter reloads the list)", () => {
    const { rerender } = render(
      <ReportsView {...base} status="ACTIONED" nextCursor="20" />,
    );
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      "/admin/cases?tab=reports&status=ACTIONED&cursor=20",
    );
    rerender(<ReportsView {...base} status="ACTIONED" nextCursor="" />);
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("labels a removed listing (Moderation UI: Removed listing is labelled)", async () => {
    vi.mocked(moderateListingAction).mockResolvedValue({
      ok: false,
      error: "Tin đăng không còn tồn tại.",
    });
    render(<ReportsView {...base} />);
    expect(screen.queryByText("Tin đăng đã bị gỡ")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Xử lý tin đăng" }));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Quyết định"), {
      target: { value: "ban" },
    });
    fireEvent.change(within(dialog).getByLabelText("Lý do"), {
      target: { value: "prohibited_item" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "Xác nhận" }));
    await waitFor(() =>
      expect(screen.getByText("Tin đăng đã bị gỡ")).toBeInTheDocument(),
    );
    expect(moderateListingAction).toHaveBeenCalledWith(
      "l-1",
      "ban",
      "prohibited_item",
      "",
    );
    expect(screen.getByRole("button", { name: "Đóng báo cáo" })).toBeEnabled();
  });

  it("Đóng báo cáo opens the close dialog (Moderation UI: Close needs a note)", () => {
    render(<ReportsView {...base} />);
    fireEvent.click(screen.getByRole("button", { name: "Đóng báo cáo" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText(/Ghi chú xử lý/)).toBeInTheDocument();
  });
});
