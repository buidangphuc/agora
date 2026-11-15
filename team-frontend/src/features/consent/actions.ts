"use server";

import { recordConsents } from "@/lib/gateway/privacy";
import { getToken } from "@/lib/gateway/session";

type Groups = {
  analytics: boolean;
  marketing: boolean;
  personalization: boolean;
};
type Prev = Groups & { version: 0 | 1 };

const PURPOSES = [
  ["analytics", "ANALYTICS"],
  ["marketing", "MARKETING"],
  ["personalization", "PERSONALIZATION"],
] as const;

export async function recordBannerConsent(
  prev: Prev,
  next: Groups,
): Promise<void> {
  if (!getToken()) return;
  const records = PURPOSES.filter(
    ([key]) => prev.version === 0 || prev[key] !== next[key],
  ).map(([key, purpose]) => ({
    purpose,
    granted: next[key],
    version: "1",
    source: "banner",
  }));
  if (records.length === 0) return;
  try {
    await recordConsents(records);
  } catch {}
}
