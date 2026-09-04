import { beforeEach, describe, expect, it, vi } from "vitest";

import { chatCopilot } from "@/lib/gateway/ai";
import {
  getOrCreateThread,
  getThreadMessages,
  markThreadRead,
  sendMessage,
} from "@/lib/gateway/chat";

import {
  chatCopilotAction,
  fetchThreadMessagesAction,
  getOrCreateThreadAction,
  markThreadReadAction,
  sendMessageAction,
} from "./actions";

vi.mock("@/lib/gateway/ai", () => ({ chatCopilot: vi.fn() }));
vi.mock("@/lib/gateway/chat", () => ({
  getOrCreateThread: vi.fn(),
  getThreadMessages: vi.fn(),
  markThreadRead: vi.fn(),
  sendMessage: vi.fn(),
}));

const thread = { id: "t1" } as never;

beforeEach(() => vi.clearAllMocks());

describe("chatCopilotAction", () => {
  it("returns AI quick replies on success", async () => {
    vi.mocked(chatCopilot).mockResolvedValue(["a", "b"]);
    const res = await chatCopilotAction("s1", "hi", "l1");
    expect(chatCopilot).toHaveBeenCalledWith("s1", "hi", "l1");
    expect(res).toEqual({ ok: true, replies: ["a", "b"] });
  });

  it("returns empty replies + message on failure", async () => {
    vi.mocked(chatCopilot).mockRejectedValue(new Error("ai down"));
    const res = await chatCopilotAction("s1", "hi");
    expect(res).toEqual({ ok: false, replies: [], message: "ai down" });
  });
});

describe("getOrCreateThreadAction", () => {
  it("returns the thread id on success", async () => {
    vi.mocked(getOrCreateThread).mockResolvedValue(thread);
    const res = await getOrCreateThreadAction("s1", "l1");
    expect(getOrCreateThread).toHaveBeenCalledWith("s1", "l1");
    expect(res).toEqual({ ok: true, threadId: "t1" });
  });

  it("returns an error message on failure", async () => {
    vi.mocked(getOrCreateThread).mockRejectedValue(new Error("nope"));
    const res = await getOrCreateThreadAction("s1", "l1");
    expect(res).toEqual({ ok: false, message: "nope" });
  });
});

describe("sendMessageAction", () => {
  it("sends directly when the thread exists", async () => {
    vi.mocked(sendMessage).mockResolvedValue({} as never);
    const res = await sendMessageAction("t1", "hello");
    expect(sendMessage).toHaveBeenCalledWith("t1", "hello");
    expect(res).toEqual({ ok: true });
  });

  it("recovers by creating a thread when the first send fails", async () => {
    vi.mocked(sendMessage)
      .mockRejectedValueOnce(new Error("no thread"))
      .mockResolvedValueOnce({} as never);
    vi.mocked(getOrCreateThread).mockResolvedValue({ id: "t9" } as never);

    const res = await sendMessageAction("seller-1", "hello", "seller-1");
    expect(getOrCreateThread).toHaveBeenCalledWith("seller-1", "");
    expect(res).toEqual({ ok: true, newThreadId: "t9" });
  });

  it("returns an error when recovery also fails", async () => {
    vi.mocked(sendMessage).mockRejectedValue(new Error("send failed"));
    vi.mocked(getOrCreateThread).mockRejectedValue(new Error("create failed"));
    const res = await sendMessageAction("t1", "hello");
    expect(res).toEqual({ ok: false, message: "create failed" });
  });
});

describe("passthrough actions", () => {
  it("markThreadReadAction swallows errors", async () => {
    vi.mocked(markThreadRead).mockRejectedValue(new Error("x"));
    await expect(markThreadReadAction("t1")).resolves.toBeUndefined();
  });

  it("fetchThreadMessagesAction returns the gateway result", async () => {
    vi.mocked(getThreadMessages).mockResolvedValue([{ id: "m1" }] as never);
    const res = await fetchThreadMessagesAction("t1");
    expect(res).toEqual([{ id: "m1" }]);
  });
});
