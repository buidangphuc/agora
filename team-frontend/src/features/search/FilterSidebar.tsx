"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
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
    currentMinPrice !== undefined ? String(currentMinPrice) : "",
  );
  const [maxPriceInput, setMaxPriceInput] = useState<string>(
    currentMaxPrice !== undefined ? String(currentMaxPrice) : "",
  );

  const priceKey = currentPriceKey(currentMinPrice, currentMaxPrice);

  function categoryName(id: string): string {
    const found = categories.find((c) => c.id === id);
    return found?.name || id;
  }

  function updateFilter(updates: {
    category?: string;
    seller?: string;
    rating?: string;
    minPrice?: string;
    maxPrice?: string;
  }) {
    const next = new URLSearchParams(searchParams.toString());
    for (const [key, val] of Object.entries(updates)) {
      if (val === undefined) {
        next.delete(key);
      } else {
        next.set(key, val);
      }
    }
    next.delete("cursor");
    router.push(`/search?${next.toString()}`);
  }

  function handleApplyPrice(e: React.FormEvent) {
    e.preventDefault();
    const min = minPriceInput.trim();
    const max = maxPriceInput.trim();
    updateFilter({
      minPrice: min && Number(min) > 0 ? min : undefined,
      maxPrice: max && Number(max) > 0 ? max : undefined,
    });
  }

  function handleClearAll() {
    setMinPriceInput("");
    setMaxPriceInput("");
    const next = new URLSearchParams(searchParams.toString());
    next.delete("category");
    next.delete("seller");
    next.delete("rating");
    next.delete("minPrice");
    next.delete("maxPrice");
    next.delete("cursor");
    router.push(`/search?${next.toString()}`);
  }

  const hasAnyFacet =
    facets.categories.length > 0 ||
    facets.priceRanges.length > 0 ||
    facets.ratings.length > 0 ||
    facets.sellers.length > 0;

  function Bucket({
    dataKey,
    label,
    count,
    active,
    onToggle,
  }: {
    dataKey: string;
    label: string;
    count: number;
    active: boolean;
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
        className={`flex w-full items-center justify-between gap-2 py-1.5 px-2 rounded-lg text-left transition text-xs cursor-pointer ${
          active
            ? "font-semibold text-primary-600 bg-primary-50/80"
            : "text-gray-700 hover:text-primary-600 hover:bg-gray-50"
        }`}
      >
        <span className="flex items-center gap-1.5 truncate">
          {active && (
            <span aria-hidden className="text-primary-600 font-bold">
              ✓
            </span>
          )}
          <span className="truncate">{label}</span>
        </span>
        <span className="shrink-0 text-xs text-gray-400">({count})</span>
      </button>
    );
  }

  return (
    <aside
      data-testid="search-facets"
      className="w-full space-y-4 lg:w-60 shrink-0 text-xs"
    >
      {/* ── Categories ── */}
      {facets.categories.length > 0 && (
        <Card
          data-testid="facet-categories"
          className="rounded-2xl p-4 border-gray-200/80 shadow-preline-card"
        >
          <h3 className="flex items-center gap-2 font-bold uppercase tracking-wider text-gray-900 border-b border-gray-100 pb-3 text-xs">
            <span>☰</span>
            <span>Danh Mục</span>
          </h3>
          <div className="mt-3 space-y-1 max-h-64 overflow-y-auto pr-1">
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
        </Card>
      )}

      <Card className="rounded-2xl p-4 border-gray-200/80 shadow-preline-card space-y-4">
        <h3 className="font-bold uppercase tracking-wider text-gray-900 flex items-center gap-2 border-b border-gray-100 pb-3 text-xs">
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
        <form
          onSubmit={handleApplyPrice}
          className="border-t border-gray-100 pt-3 space-y-2.5"
        >
          <div className="font-semibold text-gray-700">Tự Nhập Giá (₫)</div>
          <div className="flex items-center gap-2">
            <input
              type="number"
              placeholder="₫ TỪ"
              value={minPriceInput}
              onChange={(e) => setMinPriceInput(e.target.value)}
              className="w-full rounded-lg border border-gray-200 p-1.5 text-xs text-gray-900 placeholder:text-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-100"
            />
            <span className="text-gray-400">-</span>
            <input
              type="number"
              placeholder="₫ ĐẾN"
              value={maxPriceInput}
              onChange={(e) => setMaxPriceInput(e.target.value)}
              className="w-full rounded-lg border border-gray-200 p-1.5 text-xs text-gray-900 placeholder:text-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-100"
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            className="w-full font-bold uppercase tracking-wider shadow-xs"
          >
            ÁP DỤNG
          </Button>
        </form>

        {/* ── Ratings (facet) ── */}
        {facets.ratings.length > 0 && (
          <div
            data-testid="facet-ratings"
            className="border-t border-gray-100 pt-3 space-y-1"
          >
            <h4 className="font-semibold text-gray-700 mb-2">Đánh Giá</h4>
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
          <div
            data-testid="facet-sellers"
            className="border-t border-gray-100 pt-3 space-y-1"
          >
            <h4 className="font-semibold text-gray-700 mb-2">Nơi Bán</h4>
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
        <div className="pt-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleClearAll}
            className="w-full font-semibold uppercase tracking-wider text-gray-600 hover:text-gray-900"
          >
            XÓA TẤT CẢ
          </Button>
        </div>
      </Card>

      {!hasAnyFacet && (
        <p className="px-1 text-xs text-gray-400">
          Chưa có bộ lọc nào cho kết quả này.
        </p>
      )}
    </aside>
  );
}
