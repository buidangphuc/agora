import Link from "next/link";

import { Tag } from "@/components/ui/Tag";
import { focusRing } from "@/components/ui/focus";
import {
  type SearchState,
  activeFilterCount,
  buildSearchHref,
  clearFiltersHref,
} from "./url";

function formatVnd(n: number): string {
  return `${n.toLocaleString("vi-VN")}₫`;
}

/**
 * Applied filters as removable Tags. Each "×" is a link to the URL without
 * that filter (no JavaScript); "Xóa tất cả bộ lọc" clears every filter and keeps
 * the keyword. Renders nothing when no keyword or filter is active.
 */
export function ActiveFilters({
  state,
  categoryName,
  sellerName,
}: {
  state: SearchState;
  categoryName?: string;
  sellerName?: string;
}) {
  const tags: { key: string; label: string; href: string }[] = [];
  if (state.q) {
    tags.push({
      key: "q",
      label: `Từ khóa: “${state.q}”`,
      href: buildSearchHref(state, { q: "" }),
    });
  }
  if (state.category) {
    tags.push({
      key: "category",
      label: `Danh mục: ${categoryName || state.category}`,
      href: buildSearchHref(state, { category: "" }),
    });
  }
  if (state.seller) {
    tags.push({
      key: "seller",
      label: `Nơi bán: ${sellerName || state.seller}`,
      href: buildSearchHref(state, { seller: "" }),
    });
  }
  if (state.minPrice || state.maxPrice) {
    tags.push({
      key: "price",
      label: `Giá: ${state.minPrice ? formatVnd(state.minPrice) : "0₫"} - ${
        state.maxPrice ? formatVnd(state.maxPrice) : "∞"
      }`,
      href: buildSearchHref(state, {
        minPrice: undefined,
        maxPrice: undefined,
      }),
    });
  }
  if (tags.length === 0) return null;

  return (
    <div
      data-testid="active-filters"
      className="flex flex-wrap items-center gap-2 text-sm"
    >
      <span className="text-text-secondary">Đang lọc theo:</span>
      {tags.map((t) => (
        <Tag key={t.key} color="primary">
          {t.label}
          <Link
            href={t.href}
            aria-label={`Bỏ lọc ${t.label}`}
            className={`-mr-1 inline-flex h-5 w-5 items-center justify-center rounded-xs hover:bg-primary-100 ${focusRing}`}
          >
            <span aria-hidden="true">×</span>
          </Link>
        </Tag>
      ))}
      {activeFilterCount(state) > 0 && (
        <Link
          href={clearFiltersHref(state)}
          className={`inline-flex min-h-9 items-center rounded-lg px-2 text-sm text-text-secondary underline hover:text-action-primary ${focusRing}`}
        >
          Xóa tất cả bộ lọc
        </Link>
      )}
    </div>
  );
}
