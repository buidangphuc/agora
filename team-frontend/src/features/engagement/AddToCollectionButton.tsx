"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewCollection } from "@/lib/gateway/engagement";
import { addToCollectionAction, createCollectionAction } from "./actions";

/**
 * "Add to collection" control on the listing page. Opens a small popover of the
 * user's collections; picking one adds the listing to it. Can also create a new
 * collection inline. Gateway-only via server actions.
 */
export function AddToCollectionButton({
  listingId,
  initialCollections,
  loggedIn,
}: {
  listingId: string;
  initialCollections: ViewCollection[];
  loggedIn: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [collections, setCollections] =
    useState<ViewCollection[]>(initialCollections);
  const [newName, setNewName] = useState("");
  const [pending, start] = useTransition();
  const toast = useToast();

  function add(collectionId: string, label: string) {
    start(async () => {
      const res = await addToCollectionAction(collectionId, listingId);
      if (res.ok) {
        toast.success(`✓ Đã thêm vào "${label}"`);
        setOpen(false);
      } else {
        toast.error(res.message || "Thêm vào bộ sưu tập thất bại.");
      }
    });
  }

  function createAndAdd(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = newName.trim();
    if (!trimmed) return;
    start(async () => {
      const res = await createCollectionAction(trimmed);
      if (res.ok && res.collection) {
        setCollections((prev) => [res.collection as ViewCollection, ...prev]);
        setNewName("");
        add(res.collection.id, res.collection.name);
      } else {
        toast.error(res.message || "Tạo bộ sưu tập thất bại.");
      }
    });
  }

  return (
    <div className="relative inline-block">
      <button
        type="button"
        data-testid="add-to-collection"
        onClick={() => {
          if (!loggedIn) {
            toast.info("Vui lòng đăng nhập để lưu vào bộ sưu tập.");
            return;
          }
          setOpen((o) => !o);
        }}
        className="inline-flex items-center gap-1.5 rounded-lg border border-gray-300 px-3 py-2 text-xs font-medium text-gray-700 transition hover:border-brand hover:text-brand"
      >
        <span>📁</span>
        <span>Thêm vào bộ sưu tập</span>
      </button>

      {open && (
        <div className="absolute z-20 mt-2 w-60 rounded-lg border border-gray-200 bg-white p-3 shadow-lg">
          <p className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
            Chọn bộ sưu tập
          </p>
          {collections.length === 0 ? (
            <p className="mb-2 text-xs text-gray-400">
              Chưa có bộ sưu tập nào.
            </p>
          ) : (
            <ul className="mb-2 max-h-40 space-y-1 overflow-y-auto">
              {collections.map((c) => (
                <li key={c.id}>
                  <button
                    type="button"
                    disabled={pending}
                    onClick={() => add(c.id, c.name)}
                    className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-xs text-gray-800 transition hover:bg-orange-50 disabled:opacity-60"
                  >
                    <span className="flex items-center gap-1.5">
                      <span>📂</span>
                      <span>{c.name}</span>
                    </span>
                    <span className="text-[10px] text-gray-400">
                      {c.itemCount}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <form onSubmit={createAndAdd} className="flex gap-1.5 border-t pt-2">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="Bộ sưu tập mới…"
              aria-label="Tên bộ sưu tập mới"
              className="flex-1 rounded-md border border-gray-300 px-2 py-1.5 text-xs focus:border-brand focus:outline-hidden"
            />
            <button
              type="submit"
              disabled={pending || !newName.trim()}
              className="rounded-md bg-brand px-2.5 py-1.5 text-xs font-semibold text-white disabled:opacity-60"
            >
              +
            </button>
          </form>
        </div>
      )}
    </div>
  );
}
