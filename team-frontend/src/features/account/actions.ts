"use server";

import { revalidatePath } from "next/cache";

import { revokeSession } from "@/lib/gateway/sessions";

export async function revokeSessionAction(
  sessionId: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await revokeSession(sessionId);
    revalidatePath("/account/security");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Thu hồi phiên thất bại.",
    };
  }
}
