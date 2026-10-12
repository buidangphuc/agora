import "server-only";

import { createHmac } from "node:crypto";

/**
 * Pseudonymous GA4 user_id: lowercase-hex HMAC-SHA256 of the principal id.
 * Returns null when the key is unset/empty (feature disabled). Never log the
 * key or the result.
 */
export function ga4UserId(
  principalId: string,
  key: string | undefined = process.env.GA4_USER_ID_HMAC_KEY,
): string | null {
  if (!key) return null;
  return createHmac("sha256", key).update(principalId).digest("hex");
}
