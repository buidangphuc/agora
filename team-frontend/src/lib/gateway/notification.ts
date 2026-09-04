/**
 * Server-only gateway module for team-notification (NotificationService),
 * reached — like every other domain — ONLY through the gateway (ARCHITECTURE
 * Rule 1). A per-request client is built from the caller's httpOnly `session`
 * cookie, mirroring promotion.ts. No business logic lives here: this module
 * just forwards and maps proto → plain view types.
 */
import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { NotificationService } from "@/generated/platform/notification/v1/notification_connect.js";
import {
  type AlertSubscription,
  AlertType,
  DigestFrequency,
  type Notification,
  type NotificationPrefs,
  NotificationType,
} from "@/generated/platform/notification/v1/notification_pb.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

// Build the notification client per request with the caller's bearer, using the
// same single gateway hop as client.ts (kept local so notification stays a
// self-contained gateway module, like promotion.ts).
function notification() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(NotificationService, transport);
}

export { AlertType, NotificationType, DigestFrequency };

export interface ViewNotification {
  id: string;
  title: string;
  body: string;
  type: NotificationType;
  linkUrl: string;
  isRead: boolean;
  createdAt: string;
}

export interface ViewAlertSubscription {
  id: string;
  listingId: string;
  type: AlertType;
}

function tsToString(seconds?: bigint): string {
  if (!seconds) return "";
  return new Date(Number(seconds) * 1000).toLocaleDateString("vi-VN");
}

function mapNotification(n: Notification): ViewNotification {
  return {
    id: n.id,
    title: n.title,
    body: n.body,
    type: n.type,
    linkUrl: n.linkUrl,
    isRead: n.isRead,
    createdAt: tsToString(n.createdAt?.seconds),
  };
}

function mapSubscription(s: AlertSubscription): ViewAlertSubscription {
  return {
    id: s.id,
    listingId: s.listingId,
    type: s.type,
  };
}

export async function listNotifications(
  pageSize = 30,
): Promise<{ notifications: ViewNotification[]; totalUnread: number }> {
  try {
    const res = await notification().listNotifications({ pageSize });
    return {
      notifications: res.notifications.map(mapNotification),
      totalUnread: res.totalUnread,
    };
  } catch {
    return { notifications: [], totalUnread: 0 };
  }
}

// ── Price-drop / back-in-stock alert subscriptions ──────────────────────────

export async function subscribeAlert(
  listingId: string,
  type: AlertType,
): Promise<ViewAlertSubscription | null> {
  const res = await notification().subscribeAlert({ listingId, type });
  return res.subscription ? mapSubscription(res.subscription) : null;
}

export async function unsubscribeAlert(subscriptionId: string): Promise<void> {
  await notification().unsubscribeAlert({ subscriptionId });
}

export async function listAlertSubscriptions(): Promise<
  ViewAlertSubscription[]
> {
  try {
    const res = await notification().listAlertSubscriptions({});
    return res.subscriptions.map(mapSubscription);
  } catch {
    return []; // anonymous / not logged in
  }
}

// ── Notification preferences (per-type toggles + digest frequency) ──────────

export interface ViewNotificationPrefs {
  typeEnabled: Record<string, boolean>;
  digestFreq: DigestFrequency;
}

function mapPrefs(p?: NotificationPrefs): ViewNotificationPrefs {
  return {
    typeEnabled: p?.typeEnabled ?? {},
    digestFreq: p?.digestFreq ?? DigestFrequency.OFF,
  };
}

export async function getNotificationPrefs(): Promise<ViewNotificationPrefs> {
  try {
    const res = await notification().getNotificationPrefs({});
    return mapPrefs(res.prefs);
  } catch {
    return { typeEnabled: {}, digestFreq: DigestFrequency.OFF };
  }
}

export async function updateNotificationPrefs(
  typeEnabled: Record<string, boolean>,
  digestFreq: DigestFrequency,
): Promise<ViewNotificationPrefs> {
  const res = await notification().updateNotificationPrefs({
    prefs: { typeEnabled, digestFreq },
  });
  return mapPrefs(res.prefs);
}
