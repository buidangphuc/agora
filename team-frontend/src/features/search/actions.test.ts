import { beforeEach, describe, expect, it, vi } from "vitest";

const gateway = vi.hoisted(() => ({
  saveSearch: vi.fn(),
  deleteSavedSearch: vi.fn(),
  listSavedSearches: vi.fn(),
}));
vi.mock("@/lib/gateway/search", () => gateway);

import { revalidatePath } from "next/cache";

import { deleteSavedSearchAction, saveSearchAction } from "./actions";

beforeEach(() => vi.clearAllMocks());

describe("saveSearchAction", () => {
  it("returns { ok: true, data } with a message and revalidates /search", async () => {
    const saved = { id: "1", query: "ao", filtersJson: "", createdAt: "" };
    gateway.saveSearch.mockResolvedValue(saved);
    const res = await saveSearchAction("ao");
    expect(res).toEqual({ ok: true, data: saved, message: "Đã lưu tìm kiếm." });
    expect(revalidatePath).toHaveBeenCalledWith("/search");
  });

  it("rejects an empty query with an error and saves nothing", async () => {
    const res = await saveSearchAction("   ");
    expect(res.ok).toBe(false);
    expect(res.ok === false && res.error).toBeTruthy();
    expect(res.message).toBe("Nhập từ khóa trước khi lưu.");
    expect(gateway.saveSearch).not.toHaveBeenCalled();
  });

  it("returns { ok: false, error } when the gateway fails", async () => {
    gateway.saveSearch.mockRejectedValue(new Error("boom"));
    const res = await saveSearchAction("ao");
    expect(res).toEqual({ ok: false, error: "boom", message: "boom" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("deleteSavedSearchAction", () => {
  it("returns ok and revalidates", async () => {
    gateway.deleteSavedSearch.mockResolvedValue(undefined);
    const res = await deleteSavedSearchAction("1");
    expect(res.ok).toBe(true);
    expect(revalidatePath).toHaveBeenCalledWith("/search");
  });

  it("returns the failure message", async () => {
    gateway.deleteSavedSearch.mockRejectedValue(new Error("nope"));
    const res = await deleteSavedSearchAction("1");
    expect(res).toEqual({ ok: false, error: "nope", message: "nope" });
  });
});
