import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { queryAuditLog } from "@/lib/gateway/audit";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import AuditPage from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/audit", () => ({ queryAuditLog: vi.fn() }));

// Contract: default export `AuditPage({ searchParams: Promise<...> })` (async server
// component, Next 15: searchParams is a Promise), calling
// queryAuditLog(filters, cursor) once for an admin.

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };
const buyer = { id: "u-buyer", name: "Buyer", scopes: ["order.read"] };
const INVALID_RANGE =
  "Khoảng thời gian không hợp lệ: ngày kết thúc phải sau hoặc trùng ngày bắt đầu.";
const UNAVAILABLE = "Không tải được nhật ký kiểm toán. Vui lòng thử lại sau.";

const event = {
  id: "e-1",
  actorId: "u-1",
  action: "kyc.review",
  targetType: "kyc_submission",
  targetId: "s-9",
  createdAt: "2026-10-05T17:00:00.000Z",
  metadata: {},
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(queryAuditLog).mockResolvedValue({
    status: "ok",
    events: [event],
    nextCursor: "",
  } as never);
});

describe("AuditPage", () => {
  it("redirects an anonymous visitor to login (Anonymous visitor is redirected to login)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await AuditPage({ searchParams: Promise.resolve({}) })).toBeNull();
    expect(redirect).toHaveBeenCalledWith("/login");
    expect(queryAuditLog).not.toHaveBeenCalled();
  });

  it("renders 403 for a non-admin (Non-admin sees the 403 result)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(await AuditPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Tài khoản của bạn không có quyền admin để xem Nhật ký kiểm toán.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại trang chủ" }),
    ).toBeTruthy();
    expect(queryAuditLog).not.toHaveBeenCalled();
  });

  it("does not query for a non-admin with filters (Filters never reach the gateway for a non-admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(
      await AuditPage({ searchParams: Promise.resolve({ actor_id: "a-1" }) }),
    );
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(queryAuditLog).not.toHaveBeenCalled();
  });

  it("renders 403 when the gateway denies (Gateway refusal is shown as the same 403)", async () => {
    vi.mocked(queryAuditLog).mockResolvedValue({
      status: "forbidden",
    } as never);
    render(await AuditPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByText("Gateway từ chối quyền admin cho phiên đăng nhập này."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("redirects on gateway unauthenticated (Expired session is sent to login)", async () => {
    vi.mocked(queryAuditLog).mockResolvedValue({
      status: "unauthenticated",
    } as never);
    expect(await AuditPage({ searchParams: Promise.resolve({}) })).toBeNull();
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("queries once with parsed filters and cursor and renders rows (Page size is 20; All filters are forwarded)", async () => {
    render(
      await AuditPage({
        searchParams: Promise.resolve({
          target_id: "o-1",
          from: "2026-10-01",
          cursor: "20",
        }),
      }),
    );
    expect(queryAuditLog).toHaveBeenCalledTimes(1);
    const [filters, cursor] = vi.mocked(queryAuditLog).mock.calls[0];
    expect(filters.targetId).toBe("o-1");
    expect(filters.createdAfter?.toISOString()).toBe(
      "2026-09-30T17:00:00.000Z",
    );
    expect(cursor).toBe("20");
    expect(screen.getByText("kyc_submission/s-9")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Nhật ký kiểm toán" }),
    ).toBeInTheDocument();
  });

  it("shows a local error for a bad date (Unparseable date is refused locally)", async () => {
    render(
      await AuditPage({
        searchParams: Promise.resolve({ from: "2026-13-45" }),
      }),
    );
    expect(
      screen.getByText("Ngày không hợp lệ. Vui lòng chọn lại."),
    ).toBeInTheDocument();
    expect(queryAuditLog).not.toHaveBeenCalled();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows the invalid-range message and no table (End date before start date)", async () => {
    vi.mocked(queryAuditLog).mockResolvedValue({
      status: "invalid_range",
    } as never);
    render(
      await AuditPage({
        searchParams: Promise.resolve({ from: "2026-10-05", to: "2026-10-01" }),
      }),
    );
    expect(screen.getByText(INVALID_RANGE)).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.getByLabelText("Từ ngày")).toHaveValue("2026-10-05");
  });

  it("maps a rejected cursor to unavailable (Server-rejected cursor)", async () => {
    // audit.ts maps InvalidArgument to invalid_range; with no from/to the page
    // must not claim the date range is wrong.
    vi.mocked(queryAuditLog).mockResolvedValue({
      status: "invalid_range",
    } as never);
    render(
      await AuditPage({ searchParams: Promise.resolve({ cursor: "garbage" }) }),
    );
    expect(screen.getByText(UNAVAILABLE)).toBeInTheDocument();
    expect(screen.queryByText(INVALID_RANGE)).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows the unavailable message, no raw error, no table (team-audit unavailable)", async () => {
    vi.mocked(queryAuditLog).mockResolvedValue({
      status: "unavailable",
    } as never);
    const { container } = render(
      await AuditPage({ searchParams: Promise.resolve({}) }),
    );
    expect(screen.getByText(UNAVAILABLE)).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(container.innerHTML).not.toMatch(/ConnectError|dial tcp/);
  });

  it("shows the empty state when no events match (No matching events)", async () => {
    vi.mocked(queryAuditLog).mockResolvedValue({
      status: "ok",
      events: [],
      nextCursor: "",
    } as never);
    render(await AuditPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Không có sự kiện nào")).toBeInTheDocument();
  });
});
