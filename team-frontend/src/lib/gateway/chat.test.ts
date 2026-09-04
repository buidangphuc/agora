import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  getOrCreateThread,
  getThreadMessages,
  listThreads,
  markThreadRead,
  searchMessages,
  sendMessage,
} from "./chat.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubChat(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const chat = {
    getOrCreateThread: vi.fn(),
    listThreads: vi.fn(),
    getThreadMessages: vi.fn(),
    sendMessage: vi.fn(),
    markThreadRead: vi.fn(),
    searchMessages: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ chat } as never);
  return chat;
}

const protoThread = {
  id: "t1",
  buyerId: "b1",
  sellerId: "s1",
  listingId: "l1",
  listingTitle: "Item",
  listingImageUrl: "img.png",
  lastMessageText: "hi",
  unreadCountBuyer: 0,
  unreadCountSeller: 1,
  lastMessageAt: { seconds: 1_700_000_000 },
  createdAt: { seconds: 1_700_000_000 },
  updatedAt: { seconds: 1_700_000_000 },
};

const protoMessage = {
  id: "m1",
  threadId: "t1",
  senderId: "s1",
  senderName: "",
  content: "hello",
  createdAt: { seconds: 1_700_000_000 },
};

beforeEach(() => vi.clearAllMocks());

describe("chat gateway wrapper", () => {
  it("getOrCreateThread maps the request and the returned thread", async () => {
    const chat = stubChat({
      getOrCreateThread: vi.fn().mockResolvedValue({ thread: protoThread }),
    });
    const res = await getOrCreateThread("s1", "l1");
    expect(chat.getOrCreateThread).toHaveBeenCalledWith({
      sellerId: "s1",
      listingId: "l1",
    });
    expect(res).toMatchObject({
      id: "t1",
      buyerId: "b1",
      listingTitle: "Item",
    });
  });

  it("getOrCreateThread throws when no thread is returned", async () => {
    stubChat({ getOrCreateThread: vi.fn().mockResolvedValue({}) });
    await expect(getOrCreateThread("s1", "l1")).rejects.toThrow(
      "get or create thread failed",
    );
  });

  it("listThreads normalizes errors to an empty array", async () => {
    stubChat({ listThreads: vi.fn().mockRejectedValue(new Error("x")) });
    await expect(listThreads()).resolves.toEqual([]);
  });

  it("sendMessage forwards threadId/content and defaults sender name", async () => {
    const chat = stubChat({
      sendMessage: vi.fn().mockResolvedValue({ message: protoMessage }),
    });
    const res = await sendMessage("t1", "hello");
    expect(chat.sendMessage).toHaveBeenCalledWith({
      threadId: "t1",
      content: "hello",
    });
    expect(res).toMatchObject({ id: "m1", senderName: "Người dùng" });
  });

  it("getThreadMessages maps a list of messages", async () => {
    const chat = stubChat({
      getThreadMessages: vi
        .fn()
        .mockResolvedValue({ messages: [protoMessage] }),
    });
    const res = await getThreadMessages("t1");
    expect(chat.getThreadMessages).toHaveBeenCalledWith({ threadId: "t1" });
    expect(res).toHaveLength(1);
  });

  it("searchMessages forwards the query and maps matching messages", async () => {
    const chat = stubChat({
      searchMessages: vi.fn().mockResolvedValue({ messages: [protoMessage] }),
    });
    const res = await searchMessages("hello");
    expect(chat.searchMessages).toHaveBeenCalledWith({
      query: "hello",
      page: { cursor: "", pageSize: 24 },
    });
    expect(res).toHaveLength(1);
  });

  it("searchMessages short-circuits an empty query without calling the RPC", async () => {
    const chat = stubChat({ searchMessages: vi.fn() });
    await expect(searchMessages("   ")).resolves.toEqual([]);
    expect(chat.searchMessages).not.toHaveBeenCalled();
  });

  it("searchMessages normalizes errors to an empty array", async () => {
    stubChat({ searchMessages: vi.fn().mockRejectedValue(new Error("x")) });
    await expect(searchMessages("hello")).resolves.toEqual([]);
  });

  it("markThreadRead swallows errors (best-effort receipt)", async () => {
    stubChat({ markThreadRead: vi.fn().mockRejectedValue(new Error("x")) });
    await expect(markThreadRead("t1")).resolves.toBeUndefined();
  });
});
