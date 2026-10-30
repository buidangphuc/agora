import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listModerationQueue } from "@/lib/gateway/moderation";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import ListingsPage from "./page";

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

// Contract: default export `ListingsPage({ searchParams: Promise<...> })`
// (async server component, Next 15), calling listModerationQueue(cursor) once
// for an admin.

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };
const buyer = { id: "u-buyer", name: "Buyer", scopes: ["order.read"] };
const UNAVAILABLE = "Không tải được hàng chờ kiểm duyệt. Vui lòng thử lại sau.";

const item = {
  id: "l-1",
  title: "Tin chờ duyệt",
  sellerId: "s-1",
  price: 120000,
  submittedAt: "2026-10-05T17:00:00.000Z",
  screenFlags: ["keyword:thuoc la"],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listModerationQueue).mockResolvedValue({
    status: "ok",
    items: [item],
    nextCursor: "",
  } as never);
});

describe("ListingsPage", () => {
  it("redirects an anonymous visitor to login (Moderation UI: Anonymous visitor is redirected to login)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(
      await ListingsPage({ searchParams: Promise.resolve({}) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Flistings",
    );
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("redirects when only the token is missing (Moderation UI: Anonymous visitor is redirected to login)", async () => {
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(
      await ListingsPage({ searchParams: Promise.resolve({}) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Flistings",
    );
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("renders 403 for a non-admin without calling the gateway (Moderation UI: Non-admin sees the 403 result)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(await ListingsPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại trang chủ" }),
    ).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
    expect(listModerationQueue).not.toHaveBeenCalled();
  });

  it("renders 403 when the gateway denies (Moderation UI: Gateway refusal is shown as the same 403)", async () => {
    vi.mocked(listModerationQueue).mockResolvedValue({
      status: "forbidden",
    } as never);
    render(await ListingsPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(
      screen.getByText("Gateway từ chối quyền admin cho phiên đăng nhập này."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("redirects on gateway unauthenticated (Moderation UI: Expired session is sent to login)", async () => {
    vi.mocked(listModerationQueue).mockResolvedValue({
      status: "unauthenticated",
    } as never);
    expect(
      await ListingsPage({ searchParams: Promise.resolve({}) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fadmin%2Flistings",
    );
  });

  it("shows the unavailable state without raw errors (Moderation UI: Queue backend unavailable)", async () => {
    vi.mocked(listModerationQueue).mockResolvedValue({
      status: "unavailable",
    } as never);
    const { container } = render(
      await ListingsPage({ searchParams: Promise.resolve({}) }),
    );
    expect(screen.getByText(UNAVAILABLE)).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(container.innerHTML).not.toMatch(/ConnectError|dial tcp/);
  });

  it("queries once with the URL cursor and renders rows (Moderation UI: Server order is kept and page size is 20)", async () => {
    render(
      await ListingsPage({
        searchParams: Promise.resolve({ cursor: "abc" }),
      }),
    );
    expect(listModerationQueue).toHaveBeenCalledTimes(1);
    expect(vi.mocked(listModerationQueue).mock.calls[0][0]).toBe("abc");
    expect(screen.getByText("Tin chờ duyệt")).toBeInTheDocument();
  });

  it("queries with an empty cursor by default (Moderation UI: Server order is kept and page size is 20)", async () => {
    render(await ListingsPage({ searchParams: Promise.resolve({}) }));
    expect(listModerationQueue).toHaveBeenCalledTimes(1);
    expect(vi.mocked(listModerationQueue).mock.calls[0][0] ?? "").toBe("");
  });
});
