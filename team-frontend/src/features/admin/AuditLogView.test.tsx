import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AuditLogView } from "./AuditLogView";

// Contract under test (implementer creates src/features/admin/AuditLogView.tsx):
//   <AuditLogView events nextCursor params error? />
//   events: { id, actorId, action, targetType, targetId, createdAt /*ISO*/,
//             metadata: Record<string,string> }[]
//   nextCursor: string ("" = last page)
//   params: { target_type?, target_id?, actor_id?, action?, from?, to?: string }
//   error?: "invalid_range" | "unavailable" | "invalid_date"

afterEach(cleanup);

const ev = (over: Record<string, unknown> = {}) => ({
  id: "e-1",
  actorId: "u-1",
  action: "kyc.review",
  targetType: "kyc_submission",
  targetId: "s-9",
  createdAt: "2026-10-05T17:00:00.000Z",
  metadata: {} as Record<string, string>,
  ...over,
});

const base = { events: [ev()], nextCursor: "", params: {} };

describe("AuditLogView", () => {
  it("formats time in UTC+7 (Time is shown in Vietnam time)", () => {
    render(<AuditLogView {...base} />);
    const rows = screen.getAllByRole("row");
    expect(rows[1].textContent).toContain("2026-10-06 00:00:00");
  });

  it("renders one row with sorted metadata (Rows show the five columns)", () => {
    render(
      <AuditLogView
        {...base}
        events={[ev({ metadata: { user_id: "u-7", decision: "approve" } })]}
      />,
    );
    for (const h of [
      "Thời gian",
      "Người thực hiện",
      "Hành động",
      "Đối tượng",
      "Chi tiết",
    ]) {
      expect(screen.getByRole("columnheader", { name: h })).toBeInTheDocument();
    }
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    const text = rows[1].textContent ?? "";
    expect(text).toContain("u-1");
    expect(text).toContain("kyc.review");
    expect(text).toContain("kyc_submission/s-9");
    expect(text).toContain("decision=approve, user_id=u-7");
  });

  it("keeps the server order (Server order is kept)", () => {
    render(
      <AuditLogView
        {...base}
        events={[
          ev({
            id: "3",
            targetId: "third-newest",
            createdAt: "2026-10-05T03:00:00Z",
          }),
          ev({
            id: "2",
            targetId: "second",
            createdAt: "2026-10-05T02:00:00Z",
          }),
          ev({
            id: "1",
            targetId: "oldest",
            createdAt: "2026-10-05T01:00:00Z",
          }),
        ]}
      />,
    );
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining("third-newest"),
      expect.stringContaining("second"),
      expect.stringContaining("oldest"),
    ]);
  });

  it("next link keeps filters (Next page link carries cursor and filters)", () => {
    render(
      <AuditLogView
        {...base}
        nextCursor="20"
        params={{ target_id: "o-1", action: "order.force_fail" }}
      />,
    );
    const link = screen.getByRole("link", { name: "Trang sau" });
    expect(link.getAttribute("href")).toBe(
      "/admin/audit?target_id=o-1&action=order.force_fail&cursor=20",
    );
  });

  it("hides next link on last page (Last page has no next link)", () => {
    render(<AuditLogView {...base} nextCursor="" />);
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("escapes metadata values (Markup in metadata is shown as text)", () => {
    const { container } = render(
      <AuditLogView
        {...base}
        events={[ev({ metadata: { note: "<script>alert(1)</script>" } })]}
      />,
    );
    expect(container.textContent).toContain("<script>alert(1)</script>");
    expect(container.querySelectorAll("script")).toHaveLength(0);
  });

  it("prefills filters from the url (Form is pre-filled from the URL)", () => {
    const { container } = render(
      <AuditLogView
        {...base}
        params={{ actor_id: "a-1", from: "2026-10-01" }}
      />,
    );
    expect(screen.getByLabelText("Người thực hiện (ID)")).toHaveValue("a-1");
    expect(screen.getByLabelText("Từ ngày")).toHaveValue("2026-10-01");
    expect(screen.getByLabelText("Đến ngày")).toHaveValue("");
    expect(container.querySelector("form")?.method).toBe("get");
    expect(screen.getByLabelText("Loại đối tượng")).toBeInTheDocument();
    expect(screen.getByLabelText("Mã đối tượng")).toBeInTheDocument();
    expect(screen.getByLabelText("Hành động")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Lọc" })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Xóa bộ lọc" }).getAttribute("href"),
    ).toBe("/admin/audit");
  });

  it("shows the empty state (No matching events)", () => {
    render(<AuditLogView {...base} events={[]} />);
    expect(screen.getByText("Không có sự kiện nào")).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /kyc/ })).toBeNull();
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("shows the invalid-range alert, no table, form kept (End date before start date)", () => {
    render(
      <AuditLogView
        {...base}
        events={[]}
        error="invalid_range"
        params={{ from: "2026-10-05", to: "2026-10-01" }}
      />,
    );
    expect(
      within(screen.getByRole("alert")).getByText(
        "Khoảng thời gian không hợp lệ: ngày kết thúc phải sau hoặc trùng ngày bắt đầu.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("Không có sự kiện nào")).toBeNull();
    expect(screen.getByLabelText("Từ ngày")).toHaveValue("2026-10-05");
    expect(screen.getByLabelText("Đến ngày")).toHaveValue("2026-10-01");
  });

  it("shows the unavailable alert and no table (team-audit unavailable)", () => {
    render(<AuditLogView {...base} events={[]} error="unavailable" />);
    expect(
      within(screen.getByRole("alert")).getByText(
        "Không tải được nhật ký kiểm toán. Vui lòng thử lại sau.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows the local date alert and no table (Unparseable date is refused locally)", () => {
    render(
      <AuditLogView
        {...base}
        events={[]}
        error="invalid_date"
        params={{ from: "abc" }}
      />,
    );
    expect(
      within(screen.getByRole("alert")).getByText(
        "Ngày không hợp lệ. Vui lòng chọn lại.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });
});
