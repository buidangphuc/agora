"use server";

import { ConnectError } from "@connectrpc/connect";

import { type ViewAssistantReply, shoppingAssistant } from "@/lib/gateway/ai";

export interface AssistantResult {
  ok: boolean;
  message: string;
  reply?: ViewAssistantReply;
}

/** Server Action: ask the AI shopping assistant through the gateway. */
export async function askAssistantAction(
  query: string,
  previousContext: string[] = [],
): Promise<AssistantResult> {
  const q = query.trim();
  if (!q) return { ok: false, message: "Nhập câu hỏi trước." };
  try {
    const reply = await shoppingAssistant(q, previousContext);
    return { ok: true, message: "", reply };
  } catch (err) {
    const msg =
      err instanceof ConnectError ? err.message : "Không gọi được trợ lý AI.";
    return { ok: false, message: msg };
  }
}
