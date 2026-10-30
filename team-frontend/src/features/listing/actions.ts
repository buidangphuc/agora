"use server";

import { createShareLink } from "@/lib/gateway/sharing";

/**
 * Server Action: mint a short share link for a target (e.g. a listing). Wired to
 * team-sharing SharingService through the gateway. Returns the short code.
 */
export async function createShareLinkAction(
  targetType: string,
  targetId: string,
): Promise<{ ok: boolean; shortCode?: string; message?: string }> {
  try {
    const shortCode = await createShareLink(targetType, targetId);
    return { ok: true, shortCode };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Tạo liên kết chia sẻ thất bại.",
    };
  }
}
