import { beforeEach, describe, expect, it, vi } from "vitest";

import { chatCopilot, shoppingAssistant, summarizeReviews } from "./ai.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubAi(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const ai = {
    shoppingAssistant: vi.fn(),
    chatCopilot: vi.fn(),
    summarizeReviews: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ ai } as never);
  return ai;
}

beforeEach(() => vi.clearAllMocks());

describe("ai gateway wrapper", () => {
  it("shoppingAssistant maps request + response (bigint price → number)", async () => {
    const ai = stubAi({
      shoppingAssistant: vi.fn().mockResolvedValue({
        replyText: "Here you go",
        productCards: [
          {
            listingId: "l1",
            title: "Phone",
            price: 5000000n,
            currency: "VND",
            imageUrl: "img.png",
            discountRate: 10,
            ratingText: "4.5",
          },
        ],
        suggestedFollowups: ["more?"],
        requestId: "r1",
        modelVersion: "mock:v1|search-v2",
      }),
    });

    const res = await shoppingAssistant("cheap phone", ["ctx"], "u1");
    expect(ai.shoppingAssistant).toHaveBeenCalledWith({
      userId: "u1",
      message: "cheap phone",
      previousContext: ["ctx"],
    });
    expect(res.replyText).toBe("Here you go");
    expect(res.productCards[0]).toMatchObject({
      listingId: "l1",
      price: 5000000,
    });
    expect(res.suggestedFollowups).toEqual(["more?"]);
    expect(res.requestId).toBe("r1");
    expect(res.modelVersion).toBe("mock:v1|search-v2");
  });

  it("shoppingAssistant propagates RPC errors (no local catch)", async () => {
    stubAi({
      shoppingAssistant: vi.fn().mockRejectedValue(new Error("ai down")),
    });
    await expect(shoppingAssistant("q")).rejects.toThrow("ai down");
  });

  it("chatCopilot returns quick replies with the response provenance", async () => {
    const ai = stubAi({
      chatCopilot: vi.fn().mockResolvedValue({
        quickReplies: ["Yes", "No", "Maybe"],
        requestId: "c1",
        modelVersion: "mock:v1",
      }),
    });
    const res = await chatCopilot("s1", "is it available?", "l1");
    expect(ai.chatCopilot).toHaveBeenCalledWith({
      sellerId: "s1",
      buyerMessage: "is it available?",
      listingId: "l1",
    });
    expect(res).toEqual({
      quickReplies: ["Yes", "No", "Maybe"],
      requestId: "c1",
      modelVersion: "mock:v1",
    });
  });

  it("summarizeReviews returns the response provenance", async () => {
    stubAi({
      summarizeReviews: vi.fn().mockResolvedValue({
        summary: "ok",
        pros: ["a"],
        cons: [],
        sentiment: "tích cực",
        requestId: "s1",
        modelVersion: "mock:v1",
      }),
    });
    const res = await summarizeReviews("l1", [{ rating: 5, comment: "tốt" }]);
    expect(res).toMatchObject({
      summary: "ok",
      requestId: "s1",
      modelVersion: "mock:v1",
    });
  });
});
