import { redirect } from "next/navigation";

import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { parsePage } from "@/features/account/pagination";
import type { AlertSubscriptionRow } from "@/features/notification/AlertSubscriptions";
import { NotificationsView } from "@/features/notification/NotificationsView";
import { parseTab } from "@/features/notification/categories";
import { getListing } from "@/lib/gateway/listings";
import {
  getNotificationPrefs,
  listAlertSubscriptions,
  listNotifications,
} from "@/lib/gateway/notification";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Trung Tâm Thông Báo | Sàn Thương Mại Điện Tử",
  description:
    "Cập nhật trạng thái đơn hàng, ưu đãi khuyến mãi Flash Sale và tin nhắn hệ thống.",
};

// The gateway has no total for the list, so the page reads a window large
// enough to paginate in memory (20 per page).
const WINDOW = 100;

export default async function NotificationsPage({
  searchParams,
}: {
  searchParams?: { tab?: string; page?: string };
}) {
  if (!getPrincipal()) redirect("/login");

  const [{ notifications }, subs, prefs] = await Promise.all([
    listNotifications(WINDOW),
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
    <section className="mx-auto max-w-5xl space-y-4 py-2">
      <Breadcrumb
        items={[
          { label: "Tài khoản", href: "/account/addresses" },
          { label: "Thông báo" },
        ]}
      />
      <header className="space-y-1">
        <h1 className="text-2xl font-bold text-text-primary">
          Trung tâm thông báo
        </h1>
        <p className="text-sm text-text-secondary">
          Cập nhật đơn hàng, khuyến mãi, biến động giá và tình trạng kho hàng.
        </p>
      </header>
      <NotificationsView
        notifications={notifications}
        subscriptions={subscriptions}
        prefs={prefs}
        tab={parseTab(searchParams?.tab)}
        page={parsePage(searchParams?.page)}
      />
    </section>
  );
}
