import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { magicListing } from "@/lib/gateway/ai";
import {
  createListing,
  deleteListing,
  getImageUploadUrl,
  updateListing,
} from "@/lib/gateway/listings";

import {
  type SellState,
  deleteListingAction,
  getUploadUrlAction,
  magicListingAction,
  saveListingAction,
} from "./actions";

vi.mock("@/lib/gateway/ai", () => ({ magicListing: vi.fn() }));
vi.mock("@/lib/gateway/listings", () => ({
  createListing: vi.fn(),
  updateListing: vi.fn(),
  deleteListing: vi.fn(),
  getImageUploadUrl: vi.fn(),
}));

const initial: SellState = { ok: false, message: "" };

function form(fields: Record<string, string>): FormData {
  const fd = new FormData();
  for (const [k, v] of Object.entries(fields)) fd.set(k, v);
  return fd;
}

const validListing = {
  title: "Phone",
  price: "5000000",
  currency: "VND",
  status: "published",
  stock: "3",
};

beforeEach(() => vi.clearAllMocks());

describe("saveListingAction validation", () => {
  it("requires a title", async () => {
    const res = await saveListingAction(initial, form({ price: "100" }));
    expect(res).toEqual({ ok: false, message: "Tiêu đề bắt buộc." });
    expect(createListing).not.toHaveBeenCalled();
  });

  it("rejects a negative price", async () => {
    const res = await saveListingAction(
      initial,
      form({ title: "Phone", price: "-5" }),
    );
    expect(res).toEqual({ ok: false, message: "Giá không hợp lệ." });
  });
});

describe("saveListingAction create/update", () => {
  it("creates a listing when no id is present", async () => {
    vi.mocked(createListing).mockResolvedValue({
      id: "l1",
      title: "Phone",
    } as never);
    const res = await saveListingAction(initial, form(validListing));
    expect(createListing).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Phone", price: 5000000, stock: 3 }),
    );
    expect(res).toMatchObject({ ok: true, id: "l1" });
    expect(res.message).toContain("Phone");
  });

  it("updates a listing when an id is present", async () => {
    vi.mocked(updateListing).mockResolvedValue({
      id: "l1",
      title: "Phone v2",
    } as never);
    const res = await saveListingAction(
      initial,
      form({ ...validListing, id: "l1" }),
    );
    expect(updateListing).toHaveBeenCalledWith(
      "l1",
      expect.objectContaining({ title: "Phone" }),
    );
    expect(res).toMatchObject({ ok: true, id: "l1" });
  });

  it("maps a PermissionDenied ConnectError to a friendly message", async () => {
    vi.mocked(createListing).mockRejectedValue(
      new ConnectError("nope", Code.PermissionDenied),
    );
    const res = await saveListingAction(initial, form(validListing));
    expect(res).toEqual({
      ok: false,
      message: "Bạn không có quyền chỉnh sửa sản phẩm này.",
    });
  });
});

describe("magicListingAction", () => {
  it("returns the AI result when a description is generated", async () => {
    const result = {
      generatedTitle: "T",
      generatedDescription: "D",
      suggestedCategoryId: "c1",
      suggestedPriceMin: 1,
      suggestedPriceMax: 2,
      highlightTags: [],
    };
    vi.mocked(magicListing).mockResolvedValue(result);
    const res = await magicListingAction("hint");
    expect(res).toEqual({ ok: true, message: "", result });
  });

  it("falls back to a local template when the AI call throws", async () => {
    vi.mocked(magicListing).mockRejectedValue(new Error("ai down"));
    const res = await magicListingAction("Laptop");
    expect(res.ok).toBe(true);
    expect(res.result?.generatedDescription).toContain("Laptop");
    expect(res.result?.highlightTags).toContain("Chính Hãng");
  });
});

describe("getUploadUrlAction", () => {
  it("returns the presigned fields on success", async () => {
    vi.mocked(getImageUploadUrl).mockResolvedValue({
      uploadUrl: "http://put",
      imageKey: "k1",
      publicUrl: "http://pub",
    });
    const res = await getUploadUrlAction("image/png", "a.png");
    expect(getImageUploadUrl).toHaveBeenCalledWith("image/png", "a.png");
    expect(res).toMatchObject({
      ok: true,
      uploadUrl: "http://put",
      imageKey: "k1",
    });
  });

  it("returns an error shape when the gateway throws", async () => {
    vi.mocked(getImageUploadUrl).mockRejectedValue(new Error("no creds"));
    const res = await getUploadUrlAction("image/png");
    expect(res.ok).toBe(false);
    expect(res.uploadUrl).toBe("");
  });
});

describe("deleteListingAction", () => {
  it("deletes and revalidates", async () => {
    vi.mocked(deleteListing).mockResolvedValue(undefined);
    await deleteListingAction("l1");
    expect(deleteListing).toHaveBeenCalledWith("l1");
  });
});
