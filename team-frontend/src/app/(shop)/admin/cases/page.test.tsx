import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

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

beforeEach(() => {
  vi.clearAllMocks();
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
});
