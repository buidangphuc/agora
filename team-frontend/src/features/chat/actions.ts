"use server";

import { revalidatePath } from "next/cache";

import { chatCopilot } from "@/lib/gateway/ai";
import {
  getOrCreateThread,
  getThreadMessages,
  listQuickReplies,
  markThreadRead,
  searchMessages,
  sendMessage,
} from "@/lib/gateway/chat";

/** Server Action: AI copilot reply suggestions for a seller (through the gateway). */
export async function chatCopilotAction(
  sellerId: string,
  buyerMessage: string,
  listingId = "",
): Promise<{ ok: boolean; replies: string[]; message?: string }> {
  try {
    const replies = await chatCopilot(sellerId, buyerMessage, listingId);
    return { ok: true, replies };
  } catch (err) {
    return {
      ok: false,
      replies: [],
      message: err instanceof Error ? err.message : "Không lấy được gợi ý AI.",
    };
  }
}

export async function getOrCreateThreadAction(
  sellerId: string,
  listingId: string,
): Promise<{ ok: boolean; threadId?: string; message?: string }> {
  try {
    const thread = await getOrCreateThread(sellerId, listingId);
    revalidatePath("/chat");
    return { ok: true, threadId: thread.id };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Không thể tạo hội thoại.",
    };
  }
}

export async function sendMessageAction(
  threadId: string,
  content: string,
  sellerId?: string,
): Promise<{ ok: boolean; newThreadId?: string; message?: string }> {
  try {
    await sendMessage(threadId, content);
    revalidatePath(`/chat/${threadId}`);
    revalidatePath("/chat");
    return { ok: true };
  } catch {
    // If thread does not exist yet in DB (direct URL navigation), initialize thread then send
    try {
      const targetSeller = sellerId || threadId;
      const thread = await getOrCreateThread(targetSeller, "");
      await sendMessage(thread.id, content);
      revalidatePath(`/chat/${thread.id}`);
      revalidatePath("/chat");
      return { ok: true, newThreadId: thread.id };
    } catch (err: unknown) {
      return {
        ok: false,
        message: err instanceof Error ? err.message : "Không thể gửi tin nhắn.",
      };
    }
  }
}

export async function markThreadReadAction(threadId: string): Promise<void> {
  try {
    await markThreadRead(threadId);
  } catch {
    // Silent read receipt
  }
}

export async function fetchThreadMessagesAction(threadId: string) {
  return await getThreadMessages(threadId);
}

/** Server Action: full-text message search through the gateway (team-chat). */
export async function searchMessagesAction(query: string) {
  return await searchMessages(query);
}

/** Server Action: seller-configured quick-reply chips for a thread's seller. */
export async function listQuickRepliesAction(
  sellerId: string,
): Promise<string[]> {
  return await listQuickReplies(sellerId);
}
