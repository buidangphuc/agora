import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal, getToken } from "@/lib/gateway/session";
import { listUsers } from "@/lib/gateway/users";
import UsersPage, { metadata } from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/users", () => ({
  listUsers: vi.fn(),
  setUserStatus: vi.fn(),
}));
vi.mock("@/features/admin/userActions", () => ({
  setUserStatusAction: vi.fn(),
}));

// Contract: default export `UsersPage({ searchParams: Promise<...> })` (async
// server component, same gate order as admin/audit/page.tsx) calling
// listUsers(filters, cursor) once for an admin; named export `metadata`.

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };
const buyer = { id: "u-buyer", name: "Buyer", scopes: ["order.read"] };
const FORBIDDEN_TITLE = "Cần tài khoản Quản trị";
const LOGIN_WITH_QUERY = `/login?returnUrl=${encodeURIComponent("/admin/users?query=an")}`;

const user = {
  id: "u-1",
  username: "an01",
  roles: ["buyer"],
  status: "active" as const,
  statusReason: "",
  statusUntil: "",
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listUsers).mockResolvedValue({
    status: "ok",
    users: [user],
    nextCursor: "",
  } as never);
});

describe("UsersPage", () => {
  it("redirects an anonymous visitor to login (Users UI: Anonymous visitor is redirected to login)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(
      await UsersPage({ searchParams: Promise.resolve({ query: "an" }) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledWith(LOGIN_WITH_QUERY);
    expect(listUsers).not.toHaveBeenCalled();
  });

  it("renders 403 for a non-admin (Users UI: Non-admin sees the 403 result)", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer);
    render(await UsersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText(FORBIDDEN_TITLE)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Quay lại trang chủ" }),
    ).toBeTruthy();
    expect(listUsers).not.toHaveBeenCalled();
  });

  it("renders 403 when the gateway denies (Users UI: Gateway refusal is shown as the same 403)", async () => {
    vi.mocked(listUsers).mockResolvedValue({ status: "forbidden" } as never);
    render(await UsersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText(FORBIDDEN_TITLE)).toBeInTheDocument();
    expect(
      screen.getByText("Gateway từ chối quyền admin cho phiên đăng nhập này."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("renders 403 and no rows on a denied search (Users UI: Search by a principal that lost the admin scope is denied)", async () => {
    // The cookie still says admin; the gateway disagrees on a search.
    vi.mocked(listUsers).mockResolvedValue({ status: "forbidden" } as never);
    render(
      await UsersPage({ searchParams: Promise.resolve({ query: "an01" }) }),
    );
    expect(listUsers).toHaveBeenCalledTimes(1);
    expect(screen.getByText(FORBIDDEN_TITLE)).toBeInTheDocument();
    expect(screen.queryByText("an01")).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("redirects on gateway unauthenticated (Users UI: Expired session is sent to login)", async () => {
    vi.mocked(listUsers).mockResolvedValue({
      status: "unauthenticated",
    } as never);
    expect(
      await UsersPage({ searchParams: Promise.resolve({ query: "an" }) }),
    ).toBeNull();
    expect(redirect).toHaveBeenCalledWith(LOGIN_WITH_QUERY);
  });

  it("metadata sets no-referrer (Users UI: The users page sends no referrer)", () => {
    expect(metadata.referrer).toBe("no-referrer");
  });

  it("queries once with parsed filters and renders rows (Users UI: Search is forwarded as typed fields)", async () => {
    render(
      await UsersPage({
        searchParams: Promise.resolve({
          query: "an01",
          status: "suspended",
          cursor: "c2",
        }),
      }),
    );
    expect(listUsers).toHaveBeenCalledTimes(1);
    const [filters, cursor] = vi.mocked(listUsers).mock.calls[0];
    expect(filters.query).toBe("an01");
    expect(filters.status).toBe("suspended");
    expect(cursor).toBe("c2");
    expect(screen.getByText("an01")).toBeInTheDocument();
  });

  it("shows the unavailable message and keeps the search form (Users UI: Identity unavailable)", async () => {
    vi.mocked(listUsers).mockResolvedValue({ status: "unavailable" } as never);
    const { container } = render(
      await UsersPage({ searchParams: Promise.resolve({}) }),
    );
    expect(
      screen.getByText(
        "Không tải được danh sách người dùng. Vui lòng thử lại sau.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(
      container.querySelector('[data-testid="users-search"]'),
    ).not.toBeNull();
  });
});
