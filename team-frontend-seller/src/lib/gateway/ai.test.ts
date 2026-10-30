import { beforeEach, describe, expect, it, vi } from "vitest";

import { chatCopilot, magicListing, shoppingAssistant } from "./ai.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubAi(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const ai = {
    shoppingAssistant: vi.fn(),
    magicListing: vi.fn(),
    chatCopilot: vi.fn(),
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
  });

  it("shoppingAssistant propagates RPC errors (no local catch)", async () => {
    stubAi({
      shoppingAssistant: vi.fn().mockRejectedValue(new Error("ai down")),
    });
    await expect(shoppingAssistant("q")).rejects.toThrow("ai down");
  });

  it("magicListing maps hints to the request and numbers the price range", async () => {
    const ai = stubAi({
      magicListing: vi.fn().mockResolvedValue({
        generatedTitle: "T",
        generatedDescription: "D",
        suggestedCategoryId: "c1",
        suggestedPriceMin: 100n,
        suggestedPriceMax: 200n,
        highlightTags: ["a"],
      }),
    });
    const res = await magicListing("hint", "cat", "img");
    expect(ai.magicListing).toHaveBeenCalledWith({
      titleHint: "hint",
      categoryHint: "cat",
      imageUrl: "img",
    });
    expect(res).toMatchObject({
      generatedTitle: "T",
      suggestedPriceMin: 100,
      suggestedPriceMax: 200,
    });
  });

  it("chatCopilot returns the quick replies list", async () => {
    const ai = stubAi({
      chatCopilot: vi
        .fn()
        .mockResolvedValue({ quickReplies: ["Yes", "No", "Maybe"] }),
    });
    const res = await chatCopilot("s1", "is it available?", "l1");
    expect(ai.chatCopilot).toHaveBeenCalledWith({
      sellerId: "s1",
      buyerMessage: "is it available?",
      listingId: "l1",
    });
    expect(res).toEqual(["Yes", "No", "Maybe"]);
  });
});
