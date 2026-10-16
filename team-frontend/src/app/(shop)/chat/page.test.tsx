import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listThreads } from "@/lib/gateway/chat";
import { getPrincipal } from "@/lib/gateway/session";

import ChatPage from "./page";

vi.mock("@/lib/gateway/chat", () => ({
  listThreads: vi.fn(),
  getThreadMessages: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/features/chat/ChatView", () => ({ ChatView: () => null }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(null);
});

async function visit(searchParams: Record<string, string | string[]>) {
  await expect(
    ChatPage({ searchParams: Promise.resolve(searchParams) } as never),
  ).rejects.toThrow("NEXT_REDIRECT");
}

describe("/chat page guard", () => {
  it("[Query string of the guarded page is preserved with repeated keys in order] redirects to login with path and query", async () => {
    await visit({ thread: "t1", tag: ["a", "b"] });
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Fchat%3Fthread%3Dt1%26tag%3Da%26tag%3Db",
    );
    expect(listThreads).not.toHaveBeenCalled();
  });

  it("[Guest opening a protected page is sent to login with its path] no query gives bare /chat", async () => {
    await visit({});
    expect(redirect).toHaveBeenCalledWith("/login?returnUrl=%2Fchat");
  });

  it("[Query string of the guarded page is preserved] skips undefined values and encodes keys and values", async () => {
    await visit({ thread: "a b&c", gone: undefined as never });
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/chat?thread=a%20b%26c")}`,
    );
  });

  it("[Over-length path falls back to plain login] 2100-char query gives /login", async () => {
    await visit({ thread: "x".repeat(2100) });
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("[Over-length path falls back to plain login] a path plus query of exactly 2048 chars keeps its returnUrl", async () => {
    const prefix = "/chat?thread=";
    const value = "x".repeat(2048 - prefix.length);
    await visit({ thread: value });
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent(prefix + value)}`,
    );
  });
});
