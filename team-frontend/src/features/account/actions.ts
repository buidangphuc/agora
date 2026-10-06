"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import { revokeSession } from "@/lib/gateway/sessions";
import { errorMessage } from "./action-error";

export async function revokeSessionAction(
  sessionId: string,
): Promise<ActionResult> {
  try {
    await revokeSession(sessionId);
    revalidatePath("/account/security");
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Thu hồi phiên thất bại."));
  }
}
