import { beforeEach, describe, expect, it, vi } from "vitest";

import { summarizeReviews } from "./ai.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubAi(summarize: ReturnType<typeof vi.fn>) {
  vi.mocked(makeClients).mockReturnValue({
    ai: { summarizeReviews: summarize },
  } as never);
}

beforeEach(() => vi.clearAllMocks());

describe("summarizeReviews wrapper", () => {
  it("returns null (no gateway call) when there are no reviews", async () => {
    const summarize = vi.fn();
    stubAi(summarize);
    await expect(summarizeReviews("l1", [])).resolves.toBeNull();
    expect(summarize).not.toHaveBeenCalled();
  });

  it("forwards reviews and maps the summary", async () => {
    const summarize = vi.fn().mockResolvedValue({
      summary: "good",
      pros: ["fast"],
      cons: ["pricey"],
      sentiment: "positive",
    });
    stubAi(summarize);
    const res = await summarizeReviews("l1", [{ rating: 5, comment: "nice" }]);
    expect(summarize).toHaveBeenCalledWith({
      listingId: "l1",
      reviews: [{ rating: 5, comment: "nice" }],
    });
    expect(res).toEqual({
      summary: "good",
      pros: ["fast"],
      cons: ["pricey"],
      sentiment: "positive",
    });
  });

  it("normalizes gateway errors to null", async () => {
    stubAi(vi.fn().mockRejectedValue(new Error("ai down")));
    await expect(
      summarizeReviews("l1", [{ rating: 3, comment: "meh" }]),
    ).resolves.toBeNull();
  });
});
