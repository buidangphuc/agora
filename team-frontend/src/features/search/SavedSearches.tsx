"use client";

import Link from "next/link";
import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewSavedSearch } from "@/lib/gateway/search";
import { deleteSavedSearchAction, saveSearchAction } from "./actions";

/**
 * Thin "Tìm kiếm đã lưu" panel: save the current query and re-run / delete saved
 * ones. Wired to team-search through the gateway via server actions. Styling to
 * be revamped later.
 */
export function SavedSearches({
  currentQuery,
  currentFiltersJson = "",
  initialSaved,
}: {
  currentQuery: string;
  currentFiltersJson?: string;
  initialSaved: ViewSavedSearch[];
}) {
  const [saved, setSaved] = useState(initialSaved);
  const [pending, start] = useTransition();
  const toast = useToast();

  function save() {
    start(async () => {
      const res = await saveSearchAction(currentQuery, currentFiltersJson);
      if (res.ok && res.saved) {
        setSaved((prev) => [res.saved as ViewSavedSearch, ...prev]);
        toast.success("✓ Đã lưu tìm kiếm.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  function remove(id: string) {
    setSaved((prev) => prev.filter((s) => s.id !== id));
    start(async () => {
      const res = await deleteSavedSearchAction(id);
      if (!res.ok) toast.error(res.message || "Có lỗi xảy ra.");
    });
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-3 text-xs">
      <div className="mb-2 flex items-center justify-between">
        <span className="font-semibold text-gray-800">Tìm kiếm đã lưu</span>
        <button
          type="button"
          onClick={save}
          disabled={pending || !currentQuery.trim()}
          className="rounded-md bg-brand px-2.5 py-1 font-semibold text-white transition hover:opacity-90 disabled:opacity-50"
        >
          + Lưu tìm kiếm này
        </button>
      </div>
      {saved.length === 0 ? (
        <p className="text-gray-400">Chưa có tìm kiếm nào được lưu.</p>
      ) : (
        <ul className="space-y-1">
          {saved.map((s) => (
            <li key={s.id} className="flex items-center justify-between gap-2">
              <Link
                href={`/search?q=${encodeURIComponent(s.query)}`}
                className="truncate text-emerald-700 hover:underline"
              >
                {s.query || "(tất cả)"}
              </Link>
              <button
                type="button"
                onClick={() => remove(s.id)}
                disabled={pending}
                className="shrink-0 text-gray-400 hover:text-red-600 disabled:opacity-50"
              >
                Xóa
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
