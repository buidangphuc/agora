import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { shoppingAssistant } from "@/lib/gateway/ai";

import { askAssistantAction } from "./actions";

vi.mock("@/lib/gateway/ai", () => ({ shoppingAssistant: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("askAssistantAction", () => {
  it("rejects an empty query without hitting the gateway", async () => {
    const res = await askAssistantAction("   ");
    expect(res).toEqual({ ok: false, message: "Nhập câu hỏi trước." });
    expect(shoppingAssistant).not.toHaveBeenCalled();
  });

  it("returns the reply on success and trims the query", async () => {
    const reply = {
      replyText: "hi",
      productCards: [],
      suggestedFollowups: [],
    };
    vi.mocked(shoppingAssistant).mockResolvedValue(reply);
    const res = await askAssistantAction("  find phones  ", ["ctx"]);
    expect(shoppingAssistant).toHaveBeenCalledWith("find phones", ["ctx"]);
    expect(res).toEqual({ ok: true, message: "", reply });
  });

  it("surfaces a ConnectError message on failure", async () => {
    vi.mocked(shoppingAssistant).mockRejectedValue(
      new ConnectError("rate limited", Code.ResourceExhausted),
    );
    const res = await askAssistantAction("q");
    expect(res.ok).toBe(false);
    expect(res.message).toContain("rate limited");
  });

  it("uses a generic message for non-ConnectError failures", async () => {
    vi.mocked(shoppingAssistant).mockRejectedValue(new Error("boom"));
    const res = await askAssistantAction("q");
    expect(res).toEqual({ ok: false, message: "Không gọi được trợ lý AI." });
  });
});
