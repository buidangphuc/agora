"use server";

import { revalidatePath } from "next/cache";

import {
  type AlertType,
  type DigestFrequency,
  subscribeAlert,
  unsubscribeAlert,
  updateNotificationPrefs,
} from "@/lib/gateway/notification";

export async function subscribeAlertAction(
  listingId: string,
  type: AlertType,
): Promise<{ ok: boolean; subscriptionId?: string; message?: string }> {
  try {
    const sub = await subscribeAlert(listingId, type);
    revalidatePath(`/listing/${listingId}`);
    revalidatePath("/notifications");
    return { ok: true, subscriptionId: sub?.id };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Đăng ký thông báo thất bại.",
    };
  }
}

export async function unsubscribeAlertAction(
  subscriptionId: string,
  listingId?: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await unsubscribeAlert(subscriptionId);
    if (listingId) revalidatePath(`/listing/${listingId}`);
    revalidatePath("/notifications");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Hủy thông báo thất bại.",
    };
  }
}

export async function updateNotificationPrefsAction(
  typeEnabled: Record<string, boolean>,
  digestFreq: DigestFrequency,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await updateNotificationPrefs(typeEnabled, digestFreq);
    revalidatePath("/notifications");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Cập nhật tùy chọn thất bại.",
    };
  }
}
