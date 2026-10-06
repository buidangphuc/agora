import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  DigestFrequency,
  unsubscribeAlert,
  updateNotificationPrefs,
} from "@/lib/gateway/notification";

import {
  removeAlertSubscriptionAction,
  updateNotificationPrefsAction,
} from "./actions";

vi.mock("@/lib/gateway/notification", () => ({
  DigestFrequency: { OFF: 0, DAILY: 1, WEEKLY: 2 },
  subscribeAlert: vi.fn(),
  unsubscribeAlert: vi.fn(),
  updateNotificationPrefs: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("updateNotificationPrefsAction", () => {
  it("saves, revalidates /notifications and returns ok", async () => {
    vi.mocked(updateNotificationPrefs).mockResolvedValue({} as never);
    const res = await updateNotificationPrefsAction(
      { ORDER: true },
      DigestFrequency.DAILY,
    );
    expect(res).toEqual({ ok: true });
    expect(updateNotificationPrefs).toHaveBeenCalledWith(
      { ORDER: true },
      DigestFrequency.DAILY,
    );
    expect(revalidatePath).toHaveBeenCalledWith("/notifications");
  });

  it("resolves with { ok: false, error } on a gateway error", async () => {
    vi.mocked(updateNotificationPrefs).mockRejectedValue(new Error("down"));
    await expect(
      updateNotificationPrefsAction({}, DigestFrequency.OFF),
    ).resolves.toEqual({ ok: false, error: "down" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("removeAlertSubscriptionAction", () => {
  it("unsubscribes and revalidates the notifications and listing pages", async () => {
    vi.mocked(unsubscribeAlert).mockResolvedValue(undefined);
    await expect(removeAlertSubscriptionAction("s1", "l1")).resolves.toEqual({
      ok: true,
    });
    expect(revalidatePath).toHaveBeenCalledWith("/notifications");
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
  });

  it("resolves with { ok: false, error } on a gateway error", async () => {
    vi.mocked(unsubscribeAlert).mockRejectedValue(new Error("gone"));
    await expect(removeAlertSubscriptionAction("s1")).resolves.toEqual({
      ok: false,
      error: "gone",
    });
  });
});
