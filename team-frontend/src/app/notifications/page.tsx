import type { AlertSubscriptionRow } from "@/features/notification/AlertSubscriptions";
import { NotificationsView } from "@/features/notification/NotificationsView";
import { getListing } from "@/lib/gateway/listings";
import {
  getNotificationPrefs,
  listAlertSubscriptions,
  listNotifications,
} from "@/lib/gateway/notification";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Trung Tâm Thông Báo | Sàn Thương Mại Điện Tử",
  description:
    "Cập nhật trạng thái đơn hàng, ưu đãi khuyến mãi Flash Sale và tin nhắn hệ thống.",
};

export default async function NotificationsPage() {
  const [{ notifications }, subs, prefs] = await Promise.all([
    listNotifications(),
    listAlertSubscriptions(),
    getNotificationPrefs(),
  ]);

  // Resolve listing titles for the subscription rows (gateway-only).
  const subscriptions: AlertSubscriptionRow[] = await Promise.all(
    subs.map(async (s) => {
      const listing = await getListing(s.listingId);
      return {
        id: s.id,
        listingId: s.listingId,
        type: s.type,
        title: listing?.title ?? `Sản phẩm ${s.listingId}`,
      };
    }),
  );

  return (
    <section className="py-2">
      <NotificationsView
        notifications={notifications}
        subscriptions={subscriptions}
        prefs={prefs}
      />
    </section>
  );
}
