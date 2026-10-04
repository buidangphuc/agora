"use server";

import { revalidatePath } from "next/cache";

import { errorMessage } from "@/features/account/action-error";
import { type ActionResult, fail, ok } from "@/lib/action-result";
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

/**
 * Cancel one alert subscription from /notifications. Same gateway call as
 * unsubscribeAlertAction, but with the `{ ok, error? }` result shape; the older
 * action keeps its shape for the listing page's AlertToggle.
 */
export async function removeAlertSubscriptionAction(
  subscriptionId: string,
  listingId?: string,
): Promise<ActionResult> {
  try {
    await unsubscribeAlert(subscriptionId);
    if (listingId) revalidatePath(`/listing/${listingId}`);
    revalidatePath("/notifications");
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Hủy thông báo thất bại."));
  }
}

export async function updateNotificationPrefsAction(
  typeEnabled: Record<string, boolean>,
  digestFreq: DigestFrequency,
): Promise<ActionResult> {
  try {
    await updateNotificationPrefs(typeEnabled, digestFreq);
    revalidatePath("/notifications");
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Cập nhật tùy chọn thất bại."));
  }
}
