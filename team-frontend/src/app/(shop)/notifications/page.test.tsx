import { render, screen, within } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  AlertType,
  DigestFrequency,
  NotificationType,
} from "@/generated/platform/notification/v1/notification_pb.js";
import { getListing } from "@/lib/gateway/listings";
import {
  type ViewNotification,
  getNotificationPrefs,
  listAlertSubscriptions,
  listNotifications,
} from "@/lib/gateway/notification";
import { getPrincipal } from "@/lib/gateway/session";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/listings", () => ({ getListing: vi.fn() }));
vi.mock("@/lib/gateway/notification", () => ({
  listNotifications: vi.fn(),
  listAlertSubscriptions: vi.fn(),
  getNotificationPrefs: vi.fn(),
}));
vi.mock("@/features/notification/actions", () => ({
  removeAlertSubscriptionAction: vi.fn(),
  updateNotificationPrefsAction: vi.fn(),
}));

import {
  removeAlertSubscriptionAction,
  updateNotificationPrefsAction,
} from "@/features/notification/actions";
import NotificationsPage from "./page";

function note(
  i: number,
  type: NotificationType,
  isRead = true,
): ViewNotification {
  return {
    id: `n${i}`,
    title: `Thông báo ${i}`,
    body: `Nội dung ${i}`,
    type,
    linkUrl: `/x/${i}`,
    isRead,
    createdAt: "04/10/2026",
  };
}

const NOTES: ViewNotification[] = [
  note(1, NotificationType.ORDER, false),
  note(2, NotificationType.CHAT),
  note(3, NotificationType.PRICE_DROP),
  note(4, NotificationType.ORDER),
];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(listNotifications).mockResolvedValue({
    notifications: NOTES,
    totalUnread: 1,
  });
  vi.mocked(listAlertSubscriptions).mockResolvedValue([
    { id: "sub1", listingId: "l1", type: AlertType.PRICE_DROP },
  ]);
  vi.mocked(getListing).mockResolvedValue({ title: "Áo thun" } as never);
  vi.mocked(getNotificationPrefs).mockResolvedValue({
    typeEnabled: { ORDER: true, CHAT: false },
    digestFreq: DigestFrequency.OFF,
  });
});

describe("/notifications", () => {
  it("redirects an anonymous visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await NotificationsPage({});
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("keeps data-testid and data-type on notification and alert rows", async () => {
    render(await NotificationsPage({}));
    const items = screen.getAllByTestId("notification-item");
    expect(items).toHaveLength(4);
    expect(items[0]).toHaveAttribute("data-type", "order");
    expect(items[2]).toHaveAttribute("data-type", "price_drop");
    const alert = screen.getByTestId("alert-subscription");
    expect(alert).toHaveAttribute("data-type", "price_drop");
    expect(within(alert).getByRole("button", { name: "Hủy" })).toBeVisible();
  });

  it("selects the tab from ?tab= and lists only that group", async () => {
    render(await NotificationsPage({ searchParams: { tab: "order" } }));
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: /Đơn hàng/ }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      within(tabs).getByRole("link", { name: /Đơn hàng/ }),
    ).toHaveAttribute("href", "/notifications?tab=order");
    expect(screen.getAllByTestId("notification-item")).toHaveLength(2);
    expect(screen.queryByText("Thông báo 2")).toBeNull();
  });

  it("an unknown tab falls back to all", async () => {
    render(await NotificationsPage({ searchParams: { tab: "bogus" } }));
    expect(screen.getAllByTestId("notification-item")).toHaveLength(4);
    expect(
      within(screen.getByRole("navigation", { name: "Tabs" })).getByRole(
        "link",
        {
          name: /Tất cả/,
        },
      ),
    ).toHaveAttribute("aria-current", "page");
  });

  it("shows tab counts and marks unread rows with a hidden label", async () => {
    render(await NotificationsPage({}));
    const tabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(tabs).getByRole("link", { name: /Tất cả/ }),
    ).toHaveTextContent("4");
    expect(screen.getAllByText("Chưa đọc")).toHaveLength(1);
  });

  it("renders no mark-all-read control", async () => {
    render(await NotificationsPage({}));
    expect(screen.queryByText(/đánh dấu/i)).toBeNull();
    expect(screen.queryByRole("button", { name: /đã đọc/i })).toBeNull();
  });

  it("an empty tab shows Empty inside the list area", async () => {
    render(await NotificationsPage({ searchParams: { tab: "system" } }));
    expect(
      screen.getByText("Chưa có thông báo nào trong mục này."),
    ).toBeInTheDocument();
    expect(screen.queryAllByTestId("notification-item")).toHaveLength(0);
  });

  it("paginates 20 per page via ?page=", async () => {
    vi.mocked(listNotifications).mockResolvedValue({
      notifications: Array.from({ length: 45 }, (_, i) =>
        note(i, NotificationType.ORDER),
      ),
      totalUnread: 0,
    });
    render(await NotificationsPage({ searchParams: { page: "3" } }));
    expect(screen.getAllByTestId("notification-item")).toHaveLength(5);
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "href",
      "/notifications?page=2",
    );
  });
});

describe("alert subscriptions and preferences", () => {
  it("cancelling a subscription removes the row and toasts", async () => {
    const user = setupUser();
    vi.mocked(removeAlertSubscriptionAction).mockResolvedValue({ ok: true });
    render(await NotificationsPage({}));
    await user.click(screen.getByRole("button", { name: "Hủy" }));
    expect(removeAlertSubscriptionAction).toHaveBeenCalledWith("sub1", "l1");
    expect(screen.queryByTestId("alert-subscription")).toBeNull();
    expect(toast.info).toHaveBeenCalledWith("Đã hủy theo dõi thông báo.");
  });

  it("saving preferences is pending then toasts success", async () => {
    const user = setupUser();
    vi.mocked(updateNotificationPrefsAction).mockResolvedValue({ ok: true });
    render(await NotificationsPage({}));
    await user.click(screen.getByLabelText("Tin nhắn từ shop"));
    await user.click(screen.getByRole("button", { name: "Lưu tùy chọn" }));
    expect(updateNotificationPrefsAction).toHaveBeenCalledWith(
      expect.objectContaining({ ORDER: true, CHAT: true }),
      DigestFrequency.OFF,
    );
    expect(toast.success).toHaveBeenCalledWith("Đã lưu tùy chọn thông báo.");
  });

  it("a failed save toasts the error and keeps the unsaved toggles", async () => {
    const user = setupUser();
    vi.mocked(updateNotificationPrefsAction).mockResolvedValue({
      ok: false,
      error: "Lưu thất bại.",
    });
    render(await NotificationsPage({}));
    const chat = screen.getByLabelText("Tin nhắn từ shop");
    expect(chat).not.toBeChecked();
    await user.click(chat);
    await user.click(screen.getByRole("button", { name: "Lưu tùy chọn" }));
    expect(toast.error).toHaveBeenCalledWith("Lưu thất bại.");
    expect(chat).toBeChecked();
  });
});
