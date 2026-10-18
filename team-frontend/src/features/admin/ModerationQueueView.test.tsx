import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { formatPrice } from "@/components/ui/format";
import { ModerationQueueView } from "./ModerationQueueView";
import { moderateListingAction } from "./moderationActions";

// Contract under test (implementer creates src/features/admin/ModerationQueueView.tsx):
//   <ModerationQueueView items nextCursor basePath tab? />
//   items: { id, title, sellerId, price, submittedAt /*ISO*/, screenFlags: string[] }[]
//   nextCursor: "" = last page. basePath: "/admin/cases" | "/admin/listings".
//   tab?: when set, kept in the "Trang sau" href (?tab=<tab>&cursor=<c>).
//   Each row has buttons "Duyệt" (calls moderateListingAction(id,"approve","","")),
//   "Từ chối" and "Khóa" (open ModerateDialog). Everything is rendered as text.

vi.mock("./moderationActions", () => ({ moderateListingAction: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const item = (over: Record<string, unknown> = {}) => ({
  id: "l-1",
  title: "Thuốc lá lậu",
  sellerId: "s-1",
  price: 120000,
  submittedAt: "2026-10-05T17:00:00.000Z",
  screenFlags: ["keyword:thuoc la", "ai:review"],
  ...over,
});

const base = {
  items: [item()],
  nextCursor: "",
  basePath: "/admin/cases",
  tab: "queue",
};

describe("ModerationQueueView", () => {
  it("renders one row with flags and UTC+7 time (Moderation UI: Rows show title, seller, price, submission time and flags)", () => {
    render(<ModerationQueueView {...base} />);
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(2);
    const text = rows[1].textContent ?? "";
    expect(text).toContain("Thuốc lá lậu");
    expect(text).toContain("s-1");
    expect(text).toContain(formatPrice(120000));
    expect(text).toContain("2026-10-06 00:00:00");
    expect(text).toContain("keyword:thuoc la");
    expect(text).toContain("ai:review");
    for (const h of [
      "Tin đăng",
      "Người bán",
      "Giá",
      "Gửi lúc",
      "Cờ kiểm tra",
      "Thao tác",
    ]) {
      expect(screen.getByRole("columnheader", { name: h })).toBeInTheDocument();
    }
  });

  it("keeps the order it is given (Moderation UI: Rows show title, seller, price, submission time and flags)", () => {
    render(
      <ModerationQueueView
        {...base}
        items={[
          item({ id: "a", title: "Cũ nhất" }),
          item({ id: "b", title: "Giữa" }),
          item({ id: "c", title: "Mới nhất" }),
        ]}
      />,
    );
    const rows = screen.getAllByRole("row").slice(1);
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining("Cũ nhất"),
      expect.stringContaining("Giữa"),
      expect.stringContaining("Mới nhất"),
    ]);
  });

  it("next link keeps the tab and hides on the last page (Moderation UI: Next link carries the cursor)", () => {
    const { rerender } = render(
      <ModerationQueueView {...base} nextCursor="20" />,
    );
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      "/admin/cases?tab=queue&cursor=20",
    );
    rerender(<ModerationQueueView {...base} nextCursor="" />);
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("next link without a tab uses the listings path (Moderation UI: Next link carries the cursor)", () => {
    render(
      <ModerationQueueView
        items={[item()]}
        nextCursor="20"
        basePath="/admin/listings"
      />,
    );
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      "/admin/listings?cursor=20",
    );
  });

  it("shows the empty state (Moderation UI: Empty queue)", () => {
    render(<ModerationQueueView {...base} items={[]} />);
    expect(screen.getByText("Hàng chờ trống")).toBeInTheDocument();
    expect(screen.queryAllByRole("row").length).toBeLessThanOrEqual(1);
    expect(screen.queryByRole("button", { name: "Duyệt" })).toBeNull();
  });

  it("escapes titles and flags (Moderation UI: Markup in a title or flag is shown as text)", () => {
    const { container } = render(
      <ModerationQueueView
        {...base}
        items={[
          item({
            title: "<script>alert(1)</script>",
            screenFlags: ["<img src=x onerror=alert(1)>"],
          }),
        ]}
      />,
    );
    expect(screen.getByText("<script>alert(1)</script>")).toBeInTheDocument();
    expect(container.querySelectorAll("script")).toHaveLength(0);
    expect(container.querySelectorAll("img")).toHaveLength(0);
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  });

  it("Duyệt sends one approve call for the row (Moderation UI: Approve publishes and removes the row)", async () => {
    vi.mocked(moderateListingAction).mockResolvedValue({
      ok: true,
      message: "Đã duyệt tin đăng.",
    });
    render(<ModerationQueueView {...base} />);
    fireEvent.click(screen.getByRole("button", { name: "Duyệt" }));
    await waitFor(() => expect(moderateListingAction).toHaveBeenCalledTimes(1));
    expect(moderateListingAction).toHaveBeenCalledWith(
      "l-1",
      "approve",
      "",
      "",
    );
  });

  it("Từ chối and Khóa open the dialog without calling the action (Moderation UI: Reject needs a reason code)", () => {
    render(<ModerationQueueView {...base} />);
    fireEvent.click(screen.getByRole("button", { name: "Từ chối" }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(moderateListingAction).not.toHaveBeenCalled();
  });
});
