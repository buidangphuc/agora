"use client";

import Link from "next/link";
import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewCollection } from "@/lib/gateway/engagement";
import { createCollectionAction } from "./actions";

/**
 * Wishlist collections panel on the favorites page: create a named collection
 * and see existing ones. Item add/remove happens from the listing page via
 * AddToCollectionButton.
 */
export function CollectionsManager({
  initialCollections,
}: {
  initialCollections: ViewCollection[];
}) {
  const [collections, setCollections] =
    useState<ViewCollection[]>(initialCollections);
  const [name, setName] = useState("");
  const [pending, start] = useTransition();
  const toast = useToast();

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    start(async () => {
      const res = await createCollectionAction(trimmed);
      if (res.ok && res.collection) {
        setCollections((prev) => [res.collection as ViewCollection, ...prev]);
        setName("");
        toast.success(`✓ Đã tạo bộ sưu tập "${trimmed}"`);
      } else {
        toast.error(res.message || "Tạo bộ sưu tập thất bại.");
      }
    });
  }

  return (
    <div className="rounded-xl border border-gray-100 bg-white p-5 shadow-2xs space-y-4">
      <div>
        <h2 className="text-base font-bold text-gray-900 flex items-center gap-2">
          <span>📁</span>
          <span>Bộ sưu tập của tôi</span>
        </h2>
        <p className="text-xs text-gray-500 mt-0.5">
          Nhóm các sản phẩm yêu thích thành danh sách riêng để dễ theo dõi
        </p>
      </div>

      {/* Create form */}
      <form
        aria-label="Tạo bộ sưu tập"
        onSubmit={handleCreate}
        className="flex gap-2"
      >
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Tên bộ sưu tập mới…"
          aria-label="Tên bộ sưu tập"
          className="flex-1 rounded-lg border border-gray-300 px-3 py-2 text-xs text-gray-900 focus:border-brand focus:outline-hidden"
        />
        <button
          type="submit"
          disabled={pending || !name.trim()}
          className="rounded-lg bg-brand px-4 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark disabled:opacity-60"
        >
          {pending ? "Đang tạo…" : "Tạo mới"}
        </button>
      </form>

      {/* Collections list */}
      {collections.length === 0 ? (
        <p className="text-xs text-gray-400 py-2">
          Bạn chưa có bộ sưu tập nào. Tạo một bộ sưu tập để bắt đầu nhé.
        </p>
      ) : (
        <ul className="divide-y divide-gray-100">
          {collections.map((c) => (
            <li
              key={c.id}
              data-testid="collection-row"
              data-name={c.name}
              className="flex items-center justify-between py-2.5"
            >
              <Link
                href={`/favorites?collection=${c.id}`}
                className="flex items-center gap-2 text-sm font-medium text-gray-800 hover:text-brand"
              >
                <span>📂</span>
                <span>{c.name}</span>
              </Link>
              <span className="rounded bg-gray-100 px-2 py-0.5 text-[11px] font-semibold text-gray-600">
                {c.itemCount} sản phẩm
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
