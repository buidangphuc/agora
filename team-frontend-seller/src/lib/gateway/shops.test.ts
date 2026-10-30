import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "./client.js";
import { batchGetShopNames, shopLabel } from "./shops.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubListing(batchGetStorefronts: ReturnType<typeof vi.fn>) {
  vi.mocked(makeClients).mockReturnValue({
    listing: { batchGetStorefronts },
  } as never);
  return batchGetStorefronts;
}

beforeEach(() => vi.clearAllMocks());

describe("shopLabel", () => {
  it("uses the trimmed display name when present", () => {
    expect(shopLabel("abc123xyz", "  Tiem Hoa Nho ")).toBe("Tiem Hoa Nho");
  });
  it("falls back to Shop #<first 6 chars>", () => {
    expect(shopLabel("abc123xyz")).toBe("Shop #abc123");
    expect(shopLabel("abc123xyz", "")).toBe("Shop #abc123");
    expect(shopLabel("abc123xyz", "   ")).toBe("Shop #abc123");
    expect(shopLabel("abc123xyz", null)).toBe("Shop #abc123");
  });
  it("uses a short id whole and handles an empty id", () => {
    expect(shopLabel("ab")).toBe("Shop #ab");
    expect(shopLabel("")).toBe("Shop");
  });
});

describe("batchGetShopNames", () => {
  it("makes one call for two sellers and maps names", async () => {
    const rpc = stubListing(
      vi.fn().mockResolvedValue({
        shops: [
          { sellerId: "a", displayName: "Shop Alpha", slug: "a" },
          { sellerId: "b", displayName: "Shop Beta", slug: "b" },
        ],
      }),
    );
    const names = await batchGetShopNames(["a", "b"]);
    expect(rpc).toHaveBeenCalledTimes(1);
    expect(rpc).toHaveBeenCalledWith({ sellerIds: ["a", "b"] });
    expect(names.get("a")).toBe("Shop Alpha");
    expect(names.get("b")).toBe("Shop Beta");
  });

  it("dedupes ids and skips empty ones", async () => {
    const rpc = stubListing(vi.fn().mockResolvedValue({ shops: [] }));
    await batchGetShopNames(["a", "a", "", "b", "a"]);
    expect(rpc).toHaveBeenCalledTimes(1);
    expect(rpc).toHaveBeenCalledWith({ sellerIds: ["a", "b"] });
  });

  it("makes no call for no ids", async () => {
    const rpc = stubListing(vi.fn());
    expect((await batchGetShopNames([])).size).toBe(0);
    expect(rpc).not.toHaveBeenCalled();
  });

  it("omits empty display names so callers fall back", async () => {
    stubListing(
      vi.fn().mockResolvedValue({
        shops: [{ sellerId: "a", displayName: "  ", slug: "a" }],
      }),
    );
    expect((await batchGetShopNames(["a"])).has("a")).toBe(false);
  });

  it("chunks at 100 ids, one call per chunk", async () => {
    const rpc = stubListing(vi.fn().mockResolvedValue({ shops: [] }));
    const ids = Array.from({ length: 101 }, (_, i) => `s${i}`);
    await batchGetShopNames(ids);
    expect(rpc).toHaveBeenCalledTimes(2);
    expect(rpc.mock.calls[0]?.[0].sellerIds).toHaveLength(100);
    expect(rpc.mock.calls[1]?.[0].sellerIds).toHaveLength(1);
  });

  it("swallows errors including Unimplemented and returns an empty map", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    stubListing(vi.fn().mockRejectedValue(new Error("[unimplemented] nope")));
    const names = await batchGetShopNames(["a"]);
    expect(names.size).toBe(0);
  });
});
