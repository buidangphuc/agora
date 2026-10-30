import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getDispute, listDisputes } from "@/lib/gateway/disputes";
import type { ViewDispute } from "@/lib/gateway/disputes";
import { listModerationQueue } from "@/lib/gateway/moderation";
import { listReports } from "@/lib/gateway/reports";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import CasesPage from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
}));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/moderation", () => ({
  listModerationQueue: vi.fn(),
  moderateListing: vi.fn(),
}));
vi.mock("@/lib/gateway/disputes", () => ({
  listDisputes: vi.fn(),
  getDispute: vi.fn(),
  resolveDispute: vi.fn(),
  createDispute: vi.fn(),
  listOrderDisputes: vi.fn(),
}));
vi.mock("@/features/dispute/actions", () => ({
  resolveDisputeAction: vi.fn(),
  createDisputeAction: vi.fn(),
}));
vi.mock("@/lib/gateway/reports", () => ({
  listReports: vi.fn(),
  resolveReport: vi.fn(),
  createReport: vi.fn(),
}));

// Contract: default export `CasesPage({ searchParams: Promise<...> })` (async
// server component, Next 15). Reports tab calls listReports(status, cursor)
// with status in "open" | "actioned" | "dismissed"; queue tab calls
// listModerationQueue(cursor).

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };
const buyer = { id: "u-buyer", name: "Buyer", scopes: ["order.read"] };
const UNAVAILABLE = "Không tải được danh sách báo cáo. Vui lòng thử lại sau.";

const report = {
  id: "r-1",
  listingId: "l-77",
  reasonCode: "scam",
  note: "Nghi lừa đảo",
  reporterId: "u-rep",
  status: "open",
  createdAt: "2026-10-05T17:00:00.000Z",
  resolutionNote: "",
};

const disputeRow: ViewDispute = {
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
};
const DISPUTES_UNAVAILABLE =
  "Không tải được danh sách khiếu nại. Vui lòng thử lại sau.";

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listDisputes).mockResolvedValue({
    status: "ok",
    disputes: [disputeRow],
    nextCursor: "",
  });
  vi.mocked(getDispute).mockResolvedValue({
    status: "ok",
    dispute: disputeRow,
  });
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listReports).mockResolvedValue({
    status: "ok",
    items: [report],
    nextCursor: "",
  } as never);
  vi.mocked(listModerationQueue).mockResolvedValue({
    status: "ok",
    items: [],
    nextCursor: "",
  } as never);
});

describe("CasesPage", () => {
  it("redirects an anonymous visitor keeping the tab (Moderation UI: Anonymous visitor is redirected to login)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(
      await CasesPage({ searchParams: Promise.resolve({ tab: "reports" }) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledWith("/login?returnUrl=%2Fadmin%2Fcases");
    expect(listReports).not.toHaveBeenCalled();
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("renders 403 for a non-admin and calls no RPC (Moderation UI: Non-admin sees the 403 result)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(await CasesPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại trang chủ" }),
    ).toBeTruthy();
    expect(listReports).not.toHaveBeenCalled();
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("does not call the gateway for a non-admin on the queue tab (Moderation UI: Non-admin sees the 403 result)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(
      await CasesPage({ searchParams: Promise.resolve({ tab: "queue" }) }),
    );
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("renders 403 when the gateway denies (Moderation UI: Gateway refusal is shown as the same 403)", async () => {
    vi.mocked(listReports).mockResolvedValue({ status: "forbidden" } as never);
    render(await CasesPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByText("Gateway từ chối quyền admin cho phiên đăng nhập này."),
    ).toBeInTheDocument();
  });

  it("redirects on gateway unauthenticated (Moderation UI: Expired session is sent to login)", async () => {
    vi.mocked(listReports).mockResolvedValue({
      status: "unauthenticated",
    } as never);
    expect(await CasesPage({ searchParams: Promise.resolve({}) })).toBeNull();
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(vi.mocked(redirect).mock.calls[0][0]).toMatch(
      /^\/login\?returnUrl=%2Fadmin%2Fcases$/,
    );
  });

  it("defaults to the reports tab with OPEN (Moderation UI: Default tab is reports with status OPEN)", async () => {
    render(await CasesPage({ searchParams: Promise.resolve({}) }));
    expect(listReports).toHaveBeenCalledTimes(1);
    expect(vi.mocked(listReports).mock.calls[0][0]).toBe("open");
    expect(vi.mocked(listReports).mock.calls[0][1] ?? "").toBe("");
    expect(listModerationQueue).not.toHaveBeenCalled();
    expect(screen.getByRole("link", { name: "Báo cáo" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("Nghi lừa đảo")).toBeInTheDocument();
  });

  it("forwards status and cursor to the reports call (Moderation UI: Default tab is reports with status OPEN)", async () => {
    render(
      await CasesPage({
        searchParams: Promise.resolve({
          tab: "reports",
          status: "ACTIONED",
          cursor: "c9",
        }),
      }),
    );
    expect(vi.mocked(listReports).mock.calls[0]).toEqual(["actioned", "c9"]);
  });

  it("shows the queue tab through the queue gateway (Moderation UI: Default tab is reports with status OPEN)", async () => {
    render(
      await CasesPage({ searchParams: Promise.resolve({ tab: "queue" }) }),
    );
    expect(listModerationQueue).toHaveBeenCalledTimes(1);
    expect(listReports).not.toHaveBeenCalled();
    expect(
      screen.getByRole("link", { name: "Hàng chờ duyệt" }),
    ).toHaveAttribute("aria-current", "page");
  });

  it("shows the reports unavailable state and keeps the tabs (Moderation UI: Reports backend unavailable)", async () => {
    vi.mocked(listReports).mockResolvedValue({
      status: "unavailable",
    } as never);
    const { container } = render(
      await CasesPage({ searchParams: Promise.resolve({}) }),
    );
    expect(screen.getByText(UNAVAILABLE)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Báo cáo" })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Hàng chờ duyệt" }),
    ).toBeInTheDocument();
    expect(container.innerHTML).not.toMatch(/ConnectError|dial tcp/);
  });

  it("falls back on unknown tab and status (Moderation UI: Unknown tab or status falls back)", async () => {
    render(
      await CasesPage({
        searchParams: Promise.resolve({ tab: "evil", status: "bogus" }),
      }),
    );
    expect(vi.mocked(listReports).mock.calls[0][0]).toBe("open");
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("redirects an anonymous visitor on the disputes tab (Admin disputes: Anonymous visitor on the dispute tab is redirected to login)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(
      await CasesPage({ searchParams: Promise.resolve({ tab: "disputes" }) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(vi.mocked(redirect).mock.calls[0][0]).toMatch(/^\/login/);
    expect(listDisputes).not.toHaveBeenCalled();
    expect(getDispute).not.toHaveBeenCalled();
  });

  it("renders 403 on the disputes tab for a non-admin (Admin disputes: Non-admin sees the 403 result on the dispute tab)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(
      await CasesPage({
        searchParams: Promise.resolve({ tab: "disputes", dispute: "d-1" }),
      }),
    );
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(listDisputes).not.toHaveBeenCalled();
    expect(getDispute).not.toHaveBeenCalled();
    expect(screen.queryByText("Người bán từ chối vô lý")).toBeNull();
  });

  it("maps gateway PermissionDenied and Unauthenticated on the disputes tab (Admin disputes: Gateway refusal and expired session)", async () => {
    vi.mocked(listDisputes).mockResolvedValueOnce({ status: "forbidden" });
    render(
      await CasesPage({ searchParams: Promise.resolve({ tab: "disputes" }) }),
    );
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(screen.queryByText("Người bán từ chối vô lý")).toBeNull();
    expect(redirect).not.toHaveBeenCalled();

    vi.mocked(listDisputes).mockResolvedValueOnce({
      status: "unauthenticated",
    });
    expect(
      await CasesPage({ searchParams: Promise.resolve({ tab: "disputes" }) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(vi.mocked(redirect).mock.calls[0][0]).toMatch(/^\/login/);
  });

  it("lists OPEN disputes by default and marks the tab (Admin disputes: Default list is OPEN, 20 per page)", async () => {
    render(
      await CasesPage({ searchParams: Promise.resolve({ tab: "disputes" }) }),
    );
    expect(listDisputes).toHaveBeenCalledTimes(1);
    expect(vi.mocked(listDisputes).mock.calls[0][0]).toBe("OPEN");
    expect(vi.mocked(listDisputes).mock.calls[0][1] ?? "").toBe("");
    expect(listReports).not.toHaveBeenCalled();
    expect(listModerationQueue).not.toHaveBeenCalled();
    expect(screen.getByRole("link", { name: "Khiếu nại" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("Người bán từ chối vô lý")).toBeInTheDocument();
  });

  it("forwards the status filter and cursor as the gateway status name (Admin disputes: Status filter and server order)", async () => {
    render(
      await CasesPage({
        searchParams: Promise.resolve({
          tab: "disputes",
          status: "investigating",
          cursor: "c9",
        }),
      }),
    );
    expect(vi.mocked(listDisputes).mock.calls[0]).toEqual([
      "INVESTIGATING",
      "c9",
    ]);
  });

  it("falls back to OPEN on an unknown status (Admin disputes: Unknown status falls back to OPEN)", async () => {
    render(
      await CasesPage({
        searchParams: Promise.resolve({ tab: "disputes", status: "bogus" }),
      }),
    );
    expect(vi.mocked(listDisputes).mock.calls[0][0]).toBe("OPEN");
  });

  it("shows the load failure without server text (Admin disputes: Load failure)", async () => {
    vi.mocked(listDisputes).mockResolvedValue({ status: "unavailable" });
    const { container } = render(
      await CasesPage({ searchParams: Promise.resolve({ tab: "disputes" }) }),
    );
    expect(screen.getByText(DISPUTES_UNAVAILABLE)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Khiếu nại" })).toBeInTheDocument();
    expect(screen.queryByText("Người bán từ chối vô lý")).toBeNull();
    expect(container.innerHTML).not.toMatch(/ConnectError|dial tcp/);
  });

  it("deep link renders the dispute detail (Admin disputes: Deep link opens one dispute)", async () => {
    vi.mocked(listDisputes).mockResolvedValue({
      status: "ok",
      disputes: [],
      nextCursor: "",
    });
    vi.mocked(getDispute).mockResolvedValue({
      status: "ok",
      dispute: {
        ...disputeRow,
        id: "d-7",
        status: "INVESTIGATING",
        statusText: "Đang điều tra",
      },
    });
    render(
      await CasesPage({
        searchParams: Promise.resolve({ tab: "disputes", dispute: "d-7" }),
      }),
    );
    expect(getDispute).toHaveBeenCalledTimes(1);
    expect(getDispute).toHaveBeenCalledWith("d-7");
    expect(screen.getByText("Người bán từ chối vô lý")).toBeInTheDocument();
    expect(screen.getAllByText("Đang điều tra").length).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: "Giải quyết" }),
    ).toBeInTheDocument();
  });

  it("deep link to an unknown dispute says so (Admin disputes: Deep link opens one dispute)", async () => {
    vi.mocked(getDispute).mockResolvedValue({ status: "not_found" });
    render(
      await CasesPage({
        searchParams: Promise.resolve({ tab: "disputes", dispute: "d-404" }),
      }),
    );
    expect(getDispute).toHaveBeenCalledWith("d-404");
    expect(
      screen.getByText("Không tìm thấy khiếu nại này."),
    ).toBeInTheDocument();
  });
});
