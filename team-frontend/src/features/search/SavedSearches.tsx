"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { useToast } from "@/components/ui/ToastProvider";
import { focusRing } from "@/components/ui/focus";
import type { ViewSavedSearch } from "@/lib/gateway/search";
import { deleteSavedSearchAction, saveSearchAction } from "./actions";

/**
 * "Tìm kiếm đã lưu" panel: save the current query and re-run / delete saved
 * ones, through team-search via server actions. Saving shows a pending state
 * (button `isLoading`, width kept), is disabled while pending or without a
 * keyword, and ends in a success or error toast.
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
  // Explicit flags: React 18's useTransition does not track async callbacks.
  const [saving, setSaving] = useState(false);
  const [removing, setRemoving] = useState(false);
  const toast = useToast();

  async function save() {
    setSaving(true);
    try {
      const res = await saveSearchAction(currentQuery, currentFiltersJson);
      if (res.ok && res.data) {
        const created = res.data;
        setSaved((prev) => [
          created,
          ...prev.filter((s) => s.id !== created.id),
        ]);
        toast.success("Đã lưu tìm kiếm.");
      } else if (!res.ok) {
        toast.error(res.error || "Có lỗi xảy ra.");
      }
    } catch {
      toast.error("Có lỗi xảy ra.");
    } finally {
      setSaving(false);
    }
  }

  async function remove(id: string) {
    const before = saved;
    setSaved((prev) => prev.filter((s) => s.id !== id));
    setRemoving(true);
    try {
      const res = await deleteSavedSearchAction(id);
      if (res.ok) {
        toast.success("Đã xóa tìm kiếm.");
      } else {
        setSaved(before);
        toast.error(res.error || "Có lỗi xảy ra.");
      }
    } catch {
      setSaved(before);
      toast.error("Có lỗi xảy ra.");
    } finally {
      setRemoving(false);
    }
  }

  return (
    <Card className="p-3 text-sm">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="font-semibold text-text-primary">Tìm kiếm đã lưu</span>
        <Button
          size="sm"
          variant="outline"
          onClick={save}
          isLoading={saving}
          disabled={!currentQuery.trim()}
        >
          Lưu tìm kiếm này
        </Button>
      </div>
      {saved.length === 0 ? (
        <p className="text-xs text-text-secondary">
          Chưa có tìm kiếm nào được lưu.
        </p>
      ) : (
        <ul className="space-y-1">
          {saved.map((s) => (
            <li key={s.id} className="flex items-center justify-between gap-2">
              <Link
                href={`/search?q=${encodeURIComponent(s.query)}`}
                className={`truncate rounded-xs text-action-primary hover:underline ${focusRing}`}
              >
                {s.query || "(tất cả)"}
              </Link>
              <Button
                size="xs"
                variant="ghost"
                onClick={() => remove(s.id)}
                disabled={removing}
                aria-label={`Xóa tìm kiếm ${s.query}`}
              >
                Xóa
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}
