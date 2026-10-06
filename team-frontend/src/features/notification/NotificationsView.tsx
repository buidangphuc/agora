import Link from "next/link";

import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Pagination } from "@/components/ui/Pagination";
import { Tabs } from "@/components/ui/Tabs";
import { Tag } from "@/components/ui/Tag";
import { focusRing } from "@/components/ui/focus";
import { PAGE_SIZE, paginate } from "@/features/account/pagination";
import { NotificationType } from "@/generated/platform/notification/v1/notification_pb.js";
import type {
  ViewNotification,
  ViewNotificationPrefs,
} from "@/lib/gateway/notification";
import {
  type AlertSubscriptionRow,
  AlertSubscriptions,
} from "./AlertSubscriptions";
import { NotificationPrefsForm } from "./NotificationPrefsForm";
import {
  NOTIFICATION_TABS,
  type NotificationTab,
  groupLabel,
  groupOf,
} from "./categories";

function hrefFor(tab: NotificationTab, page: number): string {
  const params = new URLSearchParams();
  if (tab !== "all") params.set("tab", tab);
  if (page > 1) params.set("page", String(page));
  const query = params.toString();
  return query ? `/notifications?${query}` : "/notifications";
}

/**
 * Notification center. Server-rendered: the tab and page come from the URL and
 * the list is filtered here, so only the preferences form and the alert
 * subscriptions are client islands. There is no "mark all read" control: the
 * backend has no mark-as-read RPC yet and read state is never simulated.
 */
export function NotificationsView({
  notifications,
  subscriptions,
  prefs,
  tab,
  page,
}: {
  notifications: ViewNotification[];
  subscriptions: AlertSubscriptionRow[];
  prefs: ViewNotificationPrefs;
  tab: NotificationTab;
  page: number;
}) {
  const countOf = (id: NotificationTab) =>
    id === "all"
      ? notifications.length
      : notifications.filter((n) => groupOf(n.type) === id).length;

  const filtered =
    tab === "all"
      ? notifications
      : notifications.filter((n) => groupOf(n.type) === tab);
  const { rows, page: current } = paginate(filtered, page);

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto">
        <Tabs
          items={NOTIFICATION_TABS.map((t) => ({
            id: t.id,
            label: t.label,
            badge: countOf(t.id),
          }))}
          activeId={tab}
          hrefFor={(id) => hrefFor(id as NotificationTab, 1)}
          variant="pills"
        />
      </div>

      <Card>
        {rows.length === 0 ? (
          <Empty description="Chưa có thông báo nào trong mục này." />
        ) : (
          <ul className="divide-y divide-border-subtle">
            {rows.map((n) => (
              <li key={n.id}>
                <Link
                  href={n.linkUrl || "#"}
                  data-testid="notification-item"
                  data-type={
                    NotificationType[n.type]?.toLowerCase?.() ?? "system"
                  }
                  className={`flex items-start gap-3 p-4 transition duration-150 hover:bg-surface-muted ${focusRing} ${
                    n.isRead ? "" : "bg-primary-50"
                  }`}
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="text-sm font-semibold text-text-primary">
                        {n.title}
                      </h3>
                      <Tag>{groupLabel(groupOf(n.type))}</Tag>
                    </div>
                    <p className="text-sm text-text-secondary">{n.body}</p>
                    <p className="text-xs text-text-disabled">{n.createdAt}</p>
                  </div>
                  {!n.isRead && (
                    <>
                      <span
                        aria-hidden="true"
                        className="mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full bg-action-primary"
                      />
                      <span className="sr-only">Chưa đọc</span>
                    </>
                  )}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Pagination
        current={current}
        total={filtered.length}
        pageSize={PAGE_SIZE}
        hrefFor={(p) => hrefFor(tab, p)}
      />

      <AlertSubscriptions initial={subscriptions} />
      <NotificationPrefsForm initial={prefs} />
    </div>
  );
}
