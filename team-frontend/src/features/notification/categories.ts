import { NotificationType } from "@/generated/platform/notification/v1/notification_pb.js";

export type NotificationGroup = "order" | "chat" | "alert" | "system";
export type NotificationTab = "all" | NotificationGroup;

export const NOTIFICATION_TABS: { id: NotificationTab; label: string }[] = [
  { id: "all", label: "Tất cả" },
  { id: "order", label: "Đơn hàng" },
  { id: "chat", label: "Tin nhắn" },
  { id: "alert", label: "Giá & Kho hàng" },
  { id: "system", label: "Hệ thống" },
];

const GROUP_LABEL: Record<NotificationGroup, string> = {
  order: "Đơn hàng",
  chat: "Tin nhắn",
  alert: "Giá & Kho hàng",
  system: "Hệ thống",
};

export function groupOf(type: NotificationType): NotificationGroup {
  switch (type) {
    case NotificationType.ORDER:
      return "order";
    case NotificationType.CHAT:
      return "chat";
    case NotificationType.PROMOTION:
    case NotificationType.PRICE_DROP:
    case NotificationType.BACK_IN_STOCK:
      return "alert";
    default:
      return "system";
  }
}

export function groupLabel(group: NotificationGroup): string {
  return GROUP_LABEL[group];
}

/** `?tab=` value to a known tab; anything else is "all". */
export function parseTab(value: string | undefined): NotificationTab {
  return NOTIFICATION_TABS.some((t) => t.id === value)
    ? (value as NotificationTab)
    : "all";
}
