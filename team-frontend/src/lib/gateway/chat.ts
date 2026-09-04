import "server-only";

import {
  type ChatMessage,
  type ChatThread,
  MessageType,
} from "@/generated/platform/chat/v1/chat_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export { MessageType };

function gateway() {
  return makeClients(getToken());
}

export interface ViewChatThread {
  id: string;
  buyerId: string;
  sellerId: string;
  listingId: string;
  listingTitle: string;
  listingImageUrl: string;
  lastMessageText: string;
  lastMessageAt: string;
  unreadCountBuyer: number;
  unreadCountSeller: number;
  createdAt: string;
  updatedAt: string;
}

export interface ViewChatMessage {
  id: string;
  threadId: string;
  senderId: string;
  senderName: string;
  content: string;
  createdAt: string;
  messageType: MessageType;
  listingId: string;
  payload: string;
}

function mapThread(t: ChatThread): ViewChatThread {
  let lastMessageAt = "";
  if (t.lastMessageAt) {
    lastMessageAt = new Date(
      Number(t.lastMessageAt.seconds) * 1000,
    ).toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit" });
  }
  let createdAt = "";
  if (t.createdAt) {
    createdAt = new Date(Number(t.createdAt.seconds) * 1000).toLocaleString(
      "vi-VN",
    );
  }
  let updatedAt = "";
  if (t.updatedAt) {
    updatedAt = new Date(Number(t.updatedAt.seconds) * 1000).toLocaleString(
      "vi-VN",
    );
  }

  return {
    id: t.id,
    buyerId: t.buyerId,
    sellerId: t.sellerId,
    listingId: t.listingId,
    listingTitle: t.listingTitle,
    listingImageUrl: t.listingImageUrl,
    lastMessageText: t.lastMessageText,
    lastMessageAt,
    unreadCountBuyer: t.unreadCountBuyer,
    unreadCountSeller: t.unreadCountSeller,
    createdAt,
    updatedAt,
  };
}

function mapMessage(m: ChatMessage): ViewChatMessage {
  let createdAt = "";
  if (m.createdAt) {
    createdAt = new Date(Number(m.createdAt.seconds) * 1000).toLocaleTimeString(
      "vi-VN",
      { hour: "2-digit", minute: "2-digit" },
    );
  }
  return {
    id: m.id,
    threadId: m.threadId,
    senderId: m.senderId,
    senderName: m.senderName || "Người dùng",
    content: m.content,
    createdAt,
    messageType: m.messageType,
    listingId: m.listingId,
    payload: m.payload,
  };
}

export async function getOrCreateThread(
  sellerId: string,
  listingId: string,
): Promise<ViewChatThread> {
  const res = await gateway().chat.getOrCreateThread({
    sellerId,
    listingId,
  });
  if (!res.thread) throw new Error("get or create thread failed");
  return mapThread(res.thread);
}

export async function listThreads(): Promise<ViewChatThread[]> {
  try {
    const res = await gateway().chat.listThreads({});
    return res.threads.map(mapThread);
  } catch {
    return [];
  }
}

export async function getThreadMessages(
  threadId: string,
): Promise<ViewChatMessage[]> {
  try {
    const res = await gateway().chat.getThreadMessages({
      threadId,
    });
    return res.messages.map(mapMessage);
  } catch {
    return [];
  }
}

export async function searchMessages(
  query: string,
): Promise<ViewChatMessage[]> {
  if (!query.trim()) return [];
  try {
    const res = await gateway().chat.searchMessages({
      query,
      page: { cursor: "", pageSize: 24 },
    });
    return res.messages.map(mapMessage);
  } catch {
    return [];
  }
}

export async function sendMessage(
  threadId: string,
  content: string,
): Promise<ViewChatMessage> {
  const res = await gateway().chat.sendMessage({
    threadId,
    content,
  });
  if (!res.message) throw new Error("send message failed");
  return mapMessage(res.message);
}

export async function markThreadRead(threadId: string): Promise<void> {
  try {
    await gateway().chat.markThreadRead({ threadId });
  } catch {
    // Ignore error on mark read
  }
}

/** Seller-configured quick-reply chips shown to buyers in a thread. */
export async function listQuickReplies(sellerId: string): Promise<string[]> {
  try {
    const res = await gateway().chat.listQuickReplies({ sellerId });
    return res.quickReplies;
  } catch {
    return [];
  }
}
