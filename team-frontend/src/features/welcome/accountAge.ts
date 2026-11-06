import "server-only";

import { makeClients } from "@/lib/gateway/client";
import { getToken } from "@/lib/gateway/session";

export async function getAccountAgeMs(): Promise<number | null> {
  try {
    const res = await makeClients(getToken()).auth.getMyAccount(
      {},
      { signal: AbortSignal.timeout(2000) },
    );
    const created = res.createdAt;
    if (!created || (created.seconds === 0n && created.nanos === 0)) {
      return null;
    }
    return Date.now() - created.toDate().getTime();
  } catch {
    return null;
  }
}
