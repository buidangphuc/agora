"use client";

import { useState, useTransition } from "react";

import { useToast } from "@/components/ui/ToastProvider";
import type { ViewBundle } from "@/lib/gateway/listings";
import { createBundleAction } from "./actions";

interface SellerListingOption {
  id: string;
  title: string;
}

/**
 * Thin bundle manager: pick ≥2 of the seller's listings, set a bundle price, and
 * create a combo. Wired to team-listing through the gateway via a server action.
 * Styling to be revamped later.
 */
export function BundleManager({
  listings,
  initialBundles,
}: {
  listings: SellerListingOption[];
  initialBundles: ViewBundle[];
}) {
  const [bundles, setBundles] = useState(initialBundles);
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [pending, start] = useTransition();
  const toast = useToast();

  function toggle(id: string) {
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    );
  }

  function submit() {
    start(async () => {
      const res = await createBundleAction(title, selected, Number(price) || 0);
      if (res.ok && res.bundle) {
        setBundles((prev) => [res.bundle as ViewBundle, ...prev]);
        setTitle("");
        setPrice("");
        setSelected([]);
        toast.success("✓ Đã tạo combo.");
      } else {
        toast.error(res.message || "Có lỗi xảy ra.");
      }
    });
  }

  return (
    <div className="space-y-4">
      {/* Create form */}
      <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs space-y-3">
        <h2 className="text-sm font-bold text-gray-800">Tạo combo mới</h2>
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Tên combo"
          className="w-full rounded-xs border border-gray-300 px-3 py-2 text-xs outline-none focus:border-brand"
        />
        <input
          value={price}
          onChange={(e) => setPrice(e.target.value)}
          type="number"
          min={0}
          placeholder="Giá combo (VND)"
          className="w-full rounded-xs border border-gray-300 px-3 py-2 text-xs outline-none focus:border-brand"
        />
        <div>
          <p className="mb-1.5 text-xs font-medium text-gray-600">
            Chọn sản phẩm ({selected.length} đã chọn)
          </p>
          {listings.length === 0 ? (
            <p className="text-xs text-gray-400">Shop chưa có sản phẩm nào.</p>
          ) : (
            <ul className="max-h-48 space-y-1 overflow-y-auto rounded-xs border border-gray-100 p-2">
              {listings.map((l) => (
                <li key={l.id}>
                  <label className="flex items-center gap-2 text-xs text-gray-700">
                    <input
                      type="checkbox"
                      checked={selected.includes(l.id)}
                      onChange={() => toggle(l.id)}
                    />
                    <span className="truncate">{l.title}</span>
                  </label>
                </li>
              ))}
            </ul>
          )}
        </div>
        <button
          type="button"
          onClick={submit}
          disabled={pending}
          className="rounded-xs bg-brand px-4 py-2 text-xs font-bold text-white shadow-xs hover:bg-brand-dark disabled:opacity-50"
        >
          {pending ? "Đang tạo..." : "Tạo combo"}
        </button>
      </div>

      {/* Existing bundles */}
      <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Combo hiện có ({bundles.length})
        </h2>
        {bundles.length === 0 ? (
          <p className="text-xs text-gray-400">Chưa có combo nào.</p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {bundles.map((b) => (
              <li
                key={b.id}
                className="flex items-center justify-between gap-3 py-2.5 text-xs"
              >
                <div>
                  <p className="font-medium text-gray-800">{b.title}</p>
                  <p className="text-gray-400">
                    {b.listingIds.length} sản phẩm · {b.createdAt}
                  </p>
                </div>
                <span className="font-bold text-brand">
                  {b.bundlePrice.toLocaleString("vi-VN")} VND
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
