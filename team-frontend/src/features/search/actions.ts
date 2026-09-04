"use server";

import { revalidatePath } from "next/cache";

import {
  type ViewSavedSearch,
  deleteSavedSearch,
  listSavedSearches,
  saveSearch,
} from "@/lib/gateway/search";

export async function saveSearchAction(
  query: string,
  filtersJson = "",
): Promise<{ ok: boolean; saved?: ViewSavedSearch; message?: string }> {
  if (!query.trim()) {
    return { ok: false, message: "Nhập từ khóa trước khi lưu." };
  }
  try {
    const saved = await saveSearch(query, filtersJson);
    revalidatePath("/search");
    return { ok: true, saved };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Lưu tìm kiếm thất bại.",
    };
  }
}

export async function listSavedSearchesAction(): Promise<ViewSavedSearch[]> {
  return listSavedSearches();
}

export async function deleteSavedSearchAction(
  id: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await deleteSavedSearch(id);
    revalidatePath("/search");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Xóa tìm kiếm thất bại.",
    };
  }
}
