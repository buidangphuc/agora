import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// The row form posts to the server action; keep the server-only gateway out.
vi.mock("./userActions", () => ({
  setUserStatusAction: vi.fn(async () => ({ ok: true, message: "" })),
}));

import type { AdminUserView } from "@/lib/gateway/users";

import { UsersView } from "./UsersView";

// Contract under test (implementer creates src/features/admin/UsersView.tsx):
//   <UsersView users nextCursor filters selfId error? />
//   users: AdminUserView[] (id, username, roles, status, statusReason,
//          statusUntil ISO "" if none)
//   nextCursor: string ("" = last page)
//   filters: { query: string; status: UserStatus | undefined; cursor: string }
//   selfId: signed-in admin's user id (no action form on that row)
//   error?: "unavailable"
// data-testids: users-search (GET form), users-row (each body row),
//   users-status-form (per-row form), users-flash (result message region).

afterEach(cleanup);

const user = (over: Partial<AdminUserView> = {}): AdminUserView => ({
  id: "u-1",
  username: "an01",
  roles: ["seller", "buyer"],
  status: "suspended",
  statusReason: "fraud",
  statusUntil: "2026-10-20T10:00:00Z",
  ...over,
});

const base = {
  users: [user()],
  nextCursor: "",
  filters: { query: "", status: undefined, cursor: "" },
  selfId: "admin-1",
};

describe("UsersView", () => {
  it("renders one row with status and local until (Users UI: Rows show the user columns)", () => {
    render(<UsersView {...base} />);
    for (const h of [
      "Tên đăng nhập",
      "ID",
      "Vai trò",
      "Trạng thái",
      "Lý do",
      "Hết hạn lúc",
      "Thao tác",
    ]) {
      expect(screen.getByRole("columnheader", { name: h })).toBeInTheDocument();
    }
    expect(screen.getAllByRole("columnheader")).toHaveLength(7);
    const rows = screen.getAllByTestId("users-row");
    expect(rows).toHaveLength(1);
    const text = rows[0].textContent ?? "";
    expect(text).toContain("an01");
    expect(text).toContain("u-1");
    expect(text).toContain("seller, buyer");
    expect(text).toContain("Tạm khóa");
    expect(text).toContain("fraud");
    expect(text).toContain("2026-10-20 17:00");
  });

  it("pre-fills the search form and exposes users-search (Users UI: Next page link carries cursor and filters)", () => {
    render(
      <UsersView
        {...base}
        filters={{ query: "an01", status: "banned", cursor: "" }}
      />,
    );
    const form = screen.getByTestId("users-search");
    expect(form.getAttribute("method")?.toLowerCase()).toBe("get");
    expect(form.getAttribute("action")).toBe("/admin/users");
    expect(within(form).getByLabelText("Tên đăng nhập hoặc ID")).toHaveValue(
      "an01",
    );
    expect(within(form).getByLabelText("Trạng thái")).toHaveValue("banned");
    expect(
      within(form).getByRole("button", { name: "Tìm" }),
    ).toBeInTheDocument();
  });

  it("next link keeps filters (Users UI: Next page link carries cursor and filters)", () => {
    render(
      <UsersView
        {...base}
        nextCursor="c3"
        filters={{ query: "an", status: undefined, cursor: "c2" }}
      />,
    );
    const link = screen.getByRole("link", { name: "Trang sau" });
    const href = link.getAttribute("href") ?? "";
    expect(href.startsWith("/admin/users?")).toBe(true);
    const q = new URL(href, "http://x").searchParams;
    expect(q.get("cursor")).toBe("c3");
    expect(q.get("query")).toBe("an");
  });

  it("next link keeps the status filter (Users UI: Next page link carries cursor and filters)", () => {
    render(
      <UsersView
        {...base}
        nextCursor="c3"
        filters={{ query: "", status: "suspended", cursor: "" }}
      />,
    );
    const href =
      screen.getByRole("link", { name: "Trang sau" }).getAttribute("href") ??
      "";
    const q = new URL(href, "http://x").searchParams;
    expect(q.get("status")).toBe("suspended");
    expect(q.get("cursor")).toBe("c3");
  });

  it("no next link on the last page (Users UI: Last page has no next link)", () => {
    render(<UsersView {...base} nextCursor="" />);
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("renders markup as text (Users UI: Markup in a username is shown as text)", () => {
    const evil = "<img src=x onerror=alert(1)>";
    const { container } = render(
      <UsersView {...base} users={[user({ username: evil })]} />,
    );
    expect(screen.getAllByTestId("users-row")[0].textContent).toContain(evil);
    expect(container.querySelectorAll("img")).toHaveLength(0);
  });

  it("shows the empty state (Users UI: No matching user)", () => {
    render(<UsersView {...base} users={[]} />);
    expect(screen.getByText("Không tìm thấy người dùng nào")).toBeVisible();
    expect(screen.queryAllByTestId("users-row")).toHaveLength(0);
  });

  it("shows the list error and keeps the search form (Users UI: No matching user)", () => {
    render(<UsersView {...base} users={[]} error="unavailable" />);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Không tải được danh sách người dùng. Vui lòng thử lại sau.",
    );
    expect(screen.getByTestId("users-search")).toBeInTheDocument();
    expect(screen.queryAllByTestId("users-row")).toHaveLength(0);
  });

  it("hides actions for self and admins (Users UI: Own row and admin rows have no action form)", () => {
    render(
      <UsersView
        {...base}
        selfId="admin-1"
        users={[
          user({ id: "admin-1", username: "me", roles: ["admin"] }),
          user({ id: "admin-2", username: "other", roles: ["admin", "buyer"] }),
          user({ id: "u-3", username: "plain", roles: ["buyer"] }),
        ]}
      />,
    );
    const rows = screen.getAllByTestId("users-row");
    expect(rows).toHaveLength(3);
    expect(within(rows[0]).queryByTestId("users-status-form")).toBeNull();
    expect(within(rows[1]).queryByTestId("users-status-form")).toBeNull();
    expect(
      within(rows[2]).getByTestId("users-status-form"),
    ).toBeInTheDocument();
    expect(screen.getAllByTestId("users-status-form")).toHaveLength(1);
  });

  it("hides the form on own row even without the admin role (Users UI: Own row and admin rows have no action form)", () => {
    render(
      <UsersView
        {...base}
        selfId="u-1"
        users={[user({ id: "u-1", roles: ["buyer"] })]}
      />,
    );
    expect(screen.queryByTestId("users-status-form")).toBeNull();
  });

  it("row form offers the reason list and status choices (Users UI: Rows show the user columns)", () => {
    render(
      <UsersView {...base} users={[user({ id: "u-3", roles: ["buyer"] })]} />,
    );
    const form = screen.getByTestId("users-status-form");
    const reason = within(form).getByLabelText("Mã lý do");
    const values = Array.from(reason.querySelectorAll("option"))
      .map((o) => o.getAttribute("value"))
      .filter((v) => v);
    expect(values).toEqual([
      "policy_violation",
      "fraud",
      "counterfeit",
      "abuse",
      "scam",
      "spam",
      "other",
    ]);
    const reasonLabels = Array.from(reason.querySelectorAll("option"))
      .filter((o) => o.getAttribute("value"))
      .map((o) => o.textContent);
    expect(reasonLabels).toEqual([
      "Vi phạm chính sách",
      "Gian lận",
      "Hàng giả",
      "Lạm dụng",
      "Lừa đảo",
      "Spam",
      "Khác",
    ]);
    const status = within(form).getByLabelText("Trạng thái mới");
    const labels = Array.from(status.querySelectorAll("option")).map(
      (o) => o.textContent,
    );
    expect(labels).toEqual(["Hoạt động", "Hạn chế", "Tạm khóa", "Cấm"]);
    expect(within(form).getByLabelText("Tạm khóa đến")).toBeInTheDocument();
    expect(
      within(form).getByLabelText("Ghi chú (tối đa 500 ký tự)"),
    ).toBeInTheDocument();
    expect(
      within(form).getByRole("button", { name: "Cập nhật trạng thái" }),
    ).toBeInTheDocument();
    expect(form.querySelector('input[name="userId"]')).toHaveValue("u-3");
  });

  it("shows the h1 and the clear-filter control (Users UI: Rows show the user columns)", () => {
    render(<UsersView {...base} />);
    expect(
      screen.getByRole("heading", { level: 1, name: "Quản lý người dùng" }),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("users-search")).getByText("Xóa bộ lọc"),
    ).toBeInTheDocument();
  });

  it("filter status select offers the proposal labels (Users UI: Rows show the user columns)", () => {
    render(<UsersView {...base} />);
    const select = within(screen.getByTestId("users-search")).getByLabelText(
      "Trạng thái",
    );
    const labels = Array.from(select.querySelectorAll("option"))
      .filter((o) => o.getAttribute("value"))
      .map((o) => o.textContent);
    expect(labels).toEqual(["Hoạt động", "Hạn chế", "Tạm khóa", "Cấm"]);
  });
});
