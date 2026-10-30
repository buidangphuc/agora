import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resolveDisputeAction } from "@/features/dispute/actions";
import type { ViewDispute } from "@/lib/gateway/disputes";

import { DisputeCasesPanel } from "./DisputeCasesPanel";

const nav = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push, replace: nav.replace, refresh: vi.fn() }),
}));
vi.mock("@/features/dispute/actions", () => ({
  resolveDisputeAction: vi.fn(),
  createDisputeAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}));

// Contract under test (implementer creates src/features/admin/DisputeCasesPanel.tsx,
// a client component):
//   <DisputeCasesPanel disputes nextCursor status detail? />
//   disputes: ViewDispute[] rendered in the given order (server order, oldest first)
//   status: "open" | "investigating" | "resolved" | "rejected" (current URL filter)
//   detail?: ViewDispute (deep link, rendered with its own actions)
//   Filter links "Chờ xử lý" / "Đang điều tra" / "Đã giải quyết" / "Đã từ chối" ->
//     /admin/cases?tab=disputes&status=open|investigating|resolved|rejected
//   Next link "Trang sau": /admin/cases?tab=disputes&status=<status>&cursor=<c>
//   Action buttons per dispute: "Bắt đầu điều tra", "Cập nhật ghi chú", "Giải quyết", "Từ chối".
//   Note field: textarea labelled "Ghi chú xử lý" (sent as `resolution`).
//   Ruling field (only when the dispute has a returnId): <select> labelled "Phán quyết"
//     with values "" | "BUYER_FAVOUR" ("Người mua thắng") | "SELLER_FAVOUR" ("Người bán thắng").
//   Buyer-favour shows a confirmation with buttons "Xác nhận" and "Hủy".
//   Actions call resolveDisputeAction({ disputeId, status, resolution, ruling? }) and on
//   { ok: true } navigate with router.push(`/admin/cases?tab=disputes&dispute=<id>`);
//   on { ok: false, error } the error is shown as text and nothing navigates.

const dispute = (over: Partial<ViewDispute> = {}): ViewDispute => ({
  id: "d-1",
  orderId: "o-1",
  claimantId: "u-buyer",
  defendantId: "u-seller",
  reason: "Người bán từ chối vô lý",
  resolution: "",
  returnId: "r-1",
  evidenceUrls: [],
  status: "OPEN",
  statusText: "Đang chờ xử lý",
  ruling: "",
  rulingText: "",
  createdAt: "2026-10-20T00:00:00.000Z",
  ...over,
});

const investigating = (over: Partial<ViewDispute> = {}) =>
  dispute({ status: "INVESTIGATING", statusText: "Đang điều tra", ...over });

const base = {
  disputes: [dispute()],
  nextCursor: "",
  status: "open" as const,
};

const CONFIRM =
  "Xác nhận phán quyết người mua thắng? Yêu cầu trả hàng sẽ được duyệt; nếu đơn đã thanh toán trực tuyến, người mua sẽ được hoàn tiền.";

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

function pickRuling(value: string) {
  fireEvent.change(screen.getByLabelText("Phán quyết"), { target: { value } });
}

describe("DisputeCasesPanel", () => {
  it("keeps the server order and applies the status filter (Admin disputes: Status filter and server order)", () => {
    const rows = [
      dispute({ id: "d-a", reason: "Lý do thứ nhất" }),
      dispute({ id: "d-b", reason: "Lý do thứ hai" }),
      dispute({ id: "d-c", reason: "Lý do thứ ba" }),
    ].map((d) => ({
      ...d,
      status: "INVESTIGATING" as const,
      statusText: "Đang điều tra",
    }));
    const { container } = render(
      <DisputeCasesPanel
        disputes={rows}
        nextCursor=""
        status="investigating"
      />,
    );
    const text = container.textContent ?? "";
    const a = text.indexOf("Lý do thứ nhất");
    const b = text.indexOf("Lý do thứ hai");
    const c = text.indexOf("Lý do thứ ba");
    expect(a).toBeGreaterThanOrEqual(0);
    expect(b).toBeGreaterThan(a);
    expect(c).toBeGreaterThan(b);
    const hrefs = [
      ["Chờ xử lý", "open"],
      ["Đang điều tra", "investigating"],
      ["Đã giải quyết", "resolved"],
      ["Đã từ chối", "rejected"],
    ];
    for (const [name, s] of hrefs) {
      expect(screen.getByRole("link", { name })).toHaveAttribute(
        "href",
        `/admin/cases?tab=disputes&status=${s}`,
      );
    }
  });

  it("builds the next link from the filter and cursor (Admin disputes: Next page link carries the cursor)", () => {
    const { rerender } = render(
      <DisputeCasesPanel {...base} status="open" nextCursor="c2" />,
    );
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      "/admin/cases?tab=disputes&status=open&cursor=c2",
    );
    rerender(<DisputeCasesPanel {...base} status="open" nextCursor="" />);
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("shows the empty state (Admin disputes: Admin tab has no disputes)", () => {
    render(<DisputeCasesPanel {...base} disputes={[]} />);
    expect(screen.getByText("Không có khiếu nại nào")).toBeInTheDocument();
  });

  it("links only http and https evidence (Admin disputes: Row detail and unsafe evidence URL)", () => {
    const { container } = render(
      <DisputeCasesPanel
        {...base}
        disputes={[
          dispute({
            evidenceUrls: ["https://x.test/a.jpg", "javascript:alert(1)"],
          }),
        ]}
      />,
    );
    const safe = Array.from(container.querySelectorAll("a")).filter(
      (a) => a.getAttribute("href") === "https://x.test/a.jpg",
    );
    expect(safe).toHaveLength(1);
    expect(safe[0].getAttribute("rel")).toBe("noopener noreferrer");
    const unsafe = Array.from(container.querySelectorAll("a")).filter((a) =>
      (a.getAttribute("href") ?? "").toLowerCase().startsWith("javascript:"),
    );
    expect(unsafe).toHaveLength(0);
    expect(container.textContent).toContain("javascript:alert(1)");
  });

  it("renders dispute text as text, not markup (Admin disputes: Row detail and unsafe evidence URL)", () => {
    const { container } = render(
      <DisputeCasesPanel
        {...base}
        disputes={[dispute({ reason: "<img src=x onerror=alert(1)>" })]}
      />,
    );
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
    expect(container.querySelectorAll("img")).toHaveLength(0);
  });

  it("offers only OPEN transitions on an open dispute (Admin disputes: Start investigating)", () => {
    render(<DisputeCasesPanel {...base} />);
    expect(
      screen.getByRole("button", { name: "Bắt đầu điều tra" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Từ chối" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Giải quyết" })).toBeNull();
  });

  it("shows the dispute detail with its new status after an action (Admin disputes: Start investigating)", async () => {
    vi.mocked(resolveDisputeAction).mockResolvedValue({
      ok: true,
      data: investigating(),
    });
    render(<DisputeCasesPanel {...base} />);
    fireEvent.click(screen.getByRole("button", { name: "Bắt đầu điều tra" }));
    await waitFor(() =>
      expect(nav.push).toHaveBeenCalledWith(
        "/admin/cases?tab=disputes&dispute=d-1",
      ),
    );
    expect(resolveDisputeAction).toHaveBeenCalledTimes(1);
    expect(resolveDisputeAction).toHaveBeenCalledWith(
      expect.objectContaining({ disputeId: "d-1", status: "INVESTIGATING" }),
    );
  });

  it("offers note, resolve and reject on an investigating dispute (Admin disputes: Start investigating)", () => {
    render(
      <DisputeCasesPanel
        {...base}
        disputes={[]}
        detail={investigating()}
        status="open"
      />,
    );
    expect(
      screen.getByRole("button", { name: "Cập nhật ghi chú" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Giải quyết" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Từ chối" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Bắt đầu điều tra" }),
    ).toBeNull();
    expect(screen.getByText("Đang điều tra")).toBeInTheDocument();
  });

  it("offers no action on a closed dispute (Admin disputes: Start investigating)", () => {
    render(
      <DisputeCasesPanel
        {...base}
        disputes={[
          dispute({
            id: "d-r",
            status: "RESOLVED",
            statusText: "Đã giải quyết",
          }),
          dispute({ id: "d-j", status: "REJECTED", statusText: "Đã từ chối" }),
        ]}
        status="resolved"
      />,
    );
    for (const name of [
      "Bắt đầu điều tra",
      "Cập nhật ghi chú",
      "Giải quyết",
      "Từ chối",
    ]) {
      expect(screen.queryByRole("button", { name })).toBeNull();
    }
  });

  it("requires a ruling for a return-linked dispute (Admin disputes: Ruling is required for a return-linked dispute)", () => {
    render(
      <DisputeCasesPanel
        {...base}
        disputes={[investigating()]}
        status="investigating"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Giải quyết" }));
    expect(screen.getByText("Vui lòng chọn phán quyết.")).toBeInTheDocument();
    expect(resolveDisputeAction).not.toHaveBeenCalled();
    expect(nav.push).not.toHaveBeenCalled();
  });

  it("confirms a buyer-favour ruling before sending (Admin disputes: Buyer-favour needs confirmation)", async () => {
    vi.mocked(resolveDisputeAction).mockResolvedValue({
      ok: true,
      data: dispute({
        status: "RESOLVED",
        statusText: "Đã giải quyết",
        ruling: "BUYER_FAVOUR",
        rulingText: "Người mua thắng",
      }),
    });
    render(
      <DisputeCasesPanel
        {...base}
        disputes={[investigating()]}
        status="investigating"
      />,
    );
    fireEvent.change(screen.getByLabelText("Ghi chú xử lý"), {
      target: { value: "Hoàn tiền cho người mua" },
    });
    pickRuling("BUYER_FAVOUR");
    fireEvent.click(screen.getByRole("button", { name: "Giải quyết" }));
    expect(screen.getByText(CONFIRM)).toBeInTheDocument();
    expect(resolveDisputeAction).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Hủy" }));
    expect(screen.queryByText(CONFIRM)).toBeNull();
    expect(resolveDisputeAction).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Giải quyết" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận" }));
    await waitFor(() => expect(resolveDisputeAction).toHaveBeenCalledTimes(1));
    expect(resolveDisputeAction).toHaveBeenCalledWith({
      disputeId: "d-1",
      status: "RESOLVED",
      resolution: "Hoàn tiền cho người mua",
      ruling: "BUYER_FAVOUR",
    });
    await waitFor(() =>
      expect(nav.push).toHaveBeenCalledWith(
        "/admin/cases?tab=disputes&dispute=d-1",
      ),
    );
  });

  it("omits the ruling for a dispute without a return (Admin disputes: Dispute without a return takes no ruling)", async () => {
    vi.mocked(resolveDisputeAction).mockResolvedValue({
      ok: true,
      data: dispute({ status: "RESOLVED", statusText: "Đã giải quyết" }),
    });
    render(
      <DisputeCasesPanel
        {...base}
        disputes={[investigating({ returnId: "" })]}
        status="investigating"
      />,
    );
    expect(screen.queryByLabelText("Phán quyết")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Giải quyết" }));
    expect(screen.queryByText(CONFIRM)).toBeNull();
    await waitFor(() => expect(resolveDisputeAction).toHaveBeenCalledTimes(1));
    const arg = vi.mocked(resolveDisputeAction).mock.calls[0][0];
    expect(arg.disputeId).toBe("d-1");
    expect(arg.status).toBe("RESOLVED");
    expect(arg.ruling).toBeUndefined();
  });

  it("shows an action error and keeps the status (Admin disputes: Server refuses the transition; Audit service is down)", async () => {
    render(<DisputeCasesPanel {...base} />);
    for (const error of [
      "Không thể chuyển khiếu nại sang trạng thái này.",
      "Hệ thống kiểm toán tạm thời không khả dụng. Chưa có thay đổi nào được lưu.",
    ]) {
      vi.mocked(resolveDisputeAction).mockResolvedValueOnce({
        ok: false,
        error,
      });
      fireEvent.click(screen.getByRole("button", { name: "Bắt đầu điều tra" }));
      await waitFor(() => expect(screen.getByText(error)).toBeInTheDocument());
      expect(nav.push).not.toHaveBeenCalled();
      expect(screen.getByText("Đang chờ xử lý")).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Bắt đầu điều tra" }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Từ chối" }),
      ).toBeInTheDocument();
    }
  });

  it("is single-flight (Admin disputes: Double click on a resolve sends one request)", async () => {
    let release: (v: Awaited<ReturnType<typeof resolveDisputeAction>>) => void =
      () => {};
    vi.mocked(resolveDisputeAction).mockReturnValue(
      new Promise((resolve) => {
        release = resolve;
      }),
    );
    render(
      <DisputeCasesPanel
        {...base}
        disputes={[investigating()]}
        status="investigating"
      />,
    );
    pickRuling("BUYER_FAVOUR");
    fireEvent.click(screen.getByRole("button", { name: "Giải quyết" }));
    const confirm = screen.getByRole("button", { name: "Xác nhận" });
    fireEvent.click(confirm);
    fireEvent.click(confirm);
    expect(resolveDisputeAction).toHaveBeenCalledTimes(1);
    release({ ok: true, data: dispute({ status: "RESOLVED" }) });
    await waitFor(() => expect(nav.push).toHaveBeenCalledTimes(1));
    expect(resolveDisputeAction).toHaveBeenCalledTimes(1);
  });
});
