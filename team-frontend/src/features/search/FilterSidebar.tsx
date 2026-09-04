"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import type { ViewCategory } from "@/lib/gateway/listings";
import type { ViewFacets } from "@/lib/gateway/search";

function formatVnd(n: number): string {
  return `${n.toLocaleString("vi-VN")}₫`;
}

/** Human label for a price-range facet key like "0-100000" or "5000000-". */
function priceRangeLabel(key: string): string {
  const [rawMin, rawMax] = key.split("-");
  const min = rawMin ? Number(rawMin) : 0;
  const max = rawMax ? Number(rawMax) : undefined;
  if (!max) return `Trên ${formatVnd(min)}`;
  if (min === 0) return `Dưới ${formatVnd(max)}`;
  return `${formatVnd(min)} - ${formatVnd(max)}`;
}

/** Parse a price-range facet key into {minPrice, maxPrice} query values. */
function priceRangeParams(key: string): {
  minPrice?: string;
  maxPrice?: string;
} {
  const [rawMin, rawMax] = key.split("-");
  return {
    minPrice: rawMin && Number(rawMin) > 0 ? rawMin : undefined,
    maxPrice: rawMax ? rawMax : undefined,
  };
}

/** Canonical "min-max" key for the currently applied price filter. */
function currentPriceKey(min?: number, max?: number): string {
  return `${min ?? 0}-${max ?? ""}`;
}

export function FilterSidebar({
  facets,
  categories,
  currentCategory,
  currentSeller,
  currentRating,
  currentMinPrice,
  currentMaxPrice,
}: {
  facets: ViewFacets;
  categories: ViewCategory[];
  currentCategory?: string;
  currentSeller?: string;
  currentRating?: string;
  currentMinPrice?: number;
  currentMaxPrice?: number;
}) {
  const router = useRouter();
  const searchParams = useSearchParams();

  const [minPriceInput, setMinPriceInput] = useState<string>(
    currentMinPrice ? String(currentMinPrice) : "",
  );
  const [maxPriceInput, setMaxPriceInput] = useState<string>(
    currentMaxPrice ? String(currentMaxPrice) : "",
  );

  const categoryName = (id: string) =>
    categories.find((c) => c.id === id)?.name ?? id;

  function updateFilter(updates: Record<string, string | undefined>) {
    const params = new URLSearchParams(searchParams.toString());
    for (const [key, val] of Object.entries(updates)) {
      if (val === undefined || val === "") {
        params.delete(key);
      } else {
        params.set(key, val);
      }
    }
    router.push(`/search?${params.toString()}`);
  }

  function handleApplyPrice(e: React.FormEvent) {
    e.preventDefault();
    updateFilter({
      minPrice: minPriceInput.trim() ? minPriceInput.trim() : undefined,
      maxPrice: maxPriceInput.trim() ? maxPriceInput.trim() : undefined,
    });
  }

  function handleClearAll() {
    setMinPriceInput("");
    setMaxPriceInput("");
    router.push("/search");
  }

  const priceKey = currentPriceKey(currentMinPrice, currentMaxPrice);
  const hasAnyFacet =
    facets.categories.length > 0 ||
    facets.priceRanges.length > 0 ||
    facets.ratings.length > 0 ||
    facets.sellers.length > 0;

  // A single clickable facet bucket exposing its key + count to e2e.
  function Bucket({
    active,
    label,
    count,
    dataKey,
    onToggle,
  }: {
    active: boolean;
    label: string;
    count: number;
    dataKey: string;
    onToggle: () => void;
  }) {
    return (
      <button
        type="button"
        data-testid="facet-bucket"
        data-key={dataKey}
        data-active={active ? "true" : "false"}
        aria-pressed={active}
        onClick={onToggle}
        className={`flex w-full items-center justify-between gap-2 py-1 px-1 rounded text-left transition ${
          active ? "font-bold text-brand" : "text-gray-700 hover:text-brand"
        }`}
      >
        <span className="flex items-center gap-1.5 truncate">
          {active && <span aria-hidden>✓</span>}
          <span className="truncate">{label}</span>
        </span>
        <span className="shrink-0 text-[11px] text-gray-400">({count})</span>
      </button>
    );
  }

  return (
    <aside
      data-testid="search-facets"
      className="w-full space-y-4 lg:w-56 shrink-0 text-xs"
    >
      {/* ── Categories ── */}
      {facets.categories.length > 0 && (
        <div
          data-testid="facet-categories"
          className="rounded-xs bg-white p-3 shadow-2xs"
        >
          <h3 className="flex items-center gap-1.5 font-bold uppercase tracking-wider text-gray-800 border-b pb-2">
            <span>☰</span>
            <span>Danh Mục</span>
          </h3>
          <div className="mt-2.5 space-y-1 max-h-64 overflow-y-auto">
            {facets.categories.map((b) => (
              <Bucket
                key={b.key}
                dataKey={b.key}
                label={categoryName(b.key)}
                count={b.count}
                active={currentCategory === b.key}
                onToggle={() =>
                  updateFilter({
                    category: currentCategory === b.key ? undefined : b.key,
                  })
                }
              />
            ))}
          </div>
        </div>
      )}

      <div className="rounded-xs bg-white p-3 shadow-2xs space-y-4">
        <h3 className="font-bold uppercase tracking-wider text-gray-800 flex items-center gap-1 border-b pb-2">
          <span>🔍</span>
          <span>Bộ Lọc Tìm Kiếm</span>
        </h3>

        {/* ── Price ranges (facet) ── */}
        {facets.priceRanges.length > 0 && (
          <div data-testid="facet-price_ranges">
            <h4 className="font-semibold text-gray-700 mb-2">Khoảng Giá</h4>
            <div className="space-y-1">
              {facets.priceRanges.map((b) => {
                const p = priceRangeParams(b.key);
                return (
                  <Bucket
                    key={b.key}
                    dataKey={b.key}
                    label={priceRangeLabel(b.key)}
                    count={b.count}
                    active={priceKey === b.key}
                    onToggle={() => {
                      const active = priceKey === b.key;
                      setMinPriceInput(active ? "" : (p.minPrice ?? ""));
                      setMaxPriceInput(active ? "" : (p.maxPrice ?? ""));
                      updateFilter(
                        active
                          ? { minPrice: undefined, maxPrice: undefined }
                          : { minPrice: p.minPrice, maxPrice: p.maxPrice },
                      );
                    }}
                  />
                );
              })}
            </div>
          </div>
        )}

        {/* ── Custom price form ── */}
        <form onSubmit={handleApplyPrice} className="border-t pt-3 space-y-2">
          <div className="font-semibold text-gray-700">Tự Nhập Giá (₫)</div>
          <div className="flex items-center gap-1">
            <input
              type="number"
              placeholder="₫ TỪ"
              value={minPriceInput}
              onChange={(e) => setMinPriceInput(e.target.value)}
              className="w-full rounded-xs border border-gray-300 p-1 text-xs text-gray-800 placeholder-gray-400 focus:border-brand focus:outline-hidden"
            />
            <span className="text-gray-400">-</span>
            <input
              type="number"
              placeholder="₫ ĐẾN"
              value={maxPriceInput}
              onChange={(e) => setMaxPriceInput(e.target.value)}
              className="w-full rounded-xs border border-gray-300 p-1 text-xs text-gray-800 placeholder-gray-400 focus:border-brand focus:outline-hidden"
            />
          </div>
          <button
            type="submit"
            className="w-full rounded-xs bg-brand py-1.5 font-bold uppercase tracking-wider text-white hover:bg-brand-dark transition shadow-2xs"
          >
            ÁP DỤNG
          </button>
        </form>

        {/* ── Ratings (facet) ── */}
        {facets.ratings.length > 0 && (
          <div data-testid="facet-ratings" className="border-t pt-3 space-y-1">
            <h4 className="font-semibold text-gray-700 mb-1.5">Đánh Giá</h4>
            {facets.ratings.map((b) => {
              const star = Math.max(0, Math.min(5, Number(b.key) || 0));
              return (
                <Bucket
                  key={b.key}
                  dataKey={b.key}
                  label={`${"★".repeat(star)}${"☆".repeat(5 - star)} ${
                    star === 5 ? "5 sao" : `từ ${star} sao`
                  }`}
                  count={b.count}
                  active={currentRating === b.key}
                  onToggle={() =>
                    updateFilter({
                      rating: currentRating === b.key ? undefined : b.key,
                    })
                  }
                />
              );
            })}
          </div>
        )}

        {/* ── Sellers (facet) ── */}
        {facets.sellers.length > 0 && (
          <div data-testid="facet-sellers" className="border-t pt-3 space-y-1">
            <h4 className="font-semibold text-gray-700 mb-1.5">Nơi Bán</h4>
            {facets.sellers.map((b) => (
              <Bucket
                key={b.key}
                dataKey={b.key}
                label={b.key}
                count={b.count}
                active={currentSeller === b.key}
                onToggle={() =>
                  updateFilter({
                    seller: currentSeller === b.key ? undefined : b.key,
                  })
                }
              />
            ))}
          </div>
        )}

        {/* ── Clear all ── */}
        <button
          type="button"
          onClick={handleClearAll}
          className="w-full rounded-xs border border-gray-300 py-1.5 font-bold uppercase tracking-wider text-gray-700 hover:border-brand hover:text-brand transition"
        >
          XÓA TẤT CẢ
        </button>
      </div>

      {!hasAnyFacet && (
        <p className="px-1 text-[11px] text-gray-400">
          Chưa có bộ lọc nào cho kết quả này.
        </p>
      )}
    </aside>
  );
}
