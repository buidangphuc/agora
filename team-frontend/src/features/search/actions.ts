"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type ViewSavedSearch,
  deleteSavedSearch,
  listSavedSearches,
  saveSearch,
} from "@/lib/gateway/search";

/** ActionResult plus the legacy `message` field (kept so existing callers still work). */
export type SearchActionResult<T = undefined> = ActionResult<T> & {
  message?: string;
};

export async function saveSearchAction(
  query: string,
  filtersJson = "",
): Promise<SearchActionResult<ViewSavedSearch>> {
  if (!query.trim()) {
    const error = "Nhập từ khóa trước khi lưu.";
    return { ...fail(error), message: error };
  }
  try {
    const saved = await saveSearch(query, filtersJson);
    revalidatePath("/search");
    return { ...ok(saved), message: "Đã lưu tìm kiếm." };
  } catch (err: unknown) {
    const error = err instanceof Error ? err.message : "Lưu tìm kiếm thất bại.";
    return { ...fail(error), message: error };
  }
}

export async function listSavedSearchesAction(): Promise<ViewSavedSearch[]> {
  return listSavedSearches();
}

export async function deleteSavedSearchAction(
  id: string,
): Promise<SearchActionResult> {
  try {
    await deleteSavedSearch(id);
    revalidatePath("/search");
    return { ...ok(), message: "Đã xóa tìm kiếm." };
  } catch (err: unknown) {
    const error = err instanceof Error ? err.message : "Xóa tìm kiếm thất bại.";
    return { ...fail(error), message: error };
  }
}
