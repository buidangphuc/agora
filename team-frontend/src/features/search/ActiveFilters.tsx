import NextLink from "next/link";

import { Link } from "@/components/ui/Link";
import { focusRing } from "@/components/ui/focus";
import {
  attributeGroupLabel,
  attributeSlugLabel,
  splitAttrKey,
} from "./attributeLabels";
import {
  type SearchState,
  activeFilterCount,
  buildSearchHref,
  clearFiltersHref,
  toggleAttr,
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
  for (const [key, slugs] of Object.entries(state.attrs)) {
    for (const slug of slugs) {
      tags.push({
        key: `${key}:${slug}`,
        label: `${attributeGroupLabel(splitAttrKey(key).group)}: ${attributeSlugLabel(slug)}`,
        href: buildSearchHref(state, {
          attrs: toggleAttr(state.attrs, key, slug),
        }),
      });
    }
  }
  if (tags.length === 0) return null;

  return (
    <div
      data-testid="active-filters"
      className="flex flex-wrap items-center gap-2 text-sm"
    >
      <span className="w-full text-text-secondary sm:w-auto">
        Đang lọc theo:
      </span>
      {tags.map((t) => (
        <NextLink
          key={t.key}
          href={t.href}
          aria-label={`Bỏ lọc ${t.label}`}
          className={`inline-flex min-h-touch max-w-full items-center gap-2 rounded-full border border-border-subtle bg-surface-card px-4 text-sm font-medium text-text-primary transition duration-150 hover:border-border-strong hover:bg-surface-muted ${focusRing}`}
        >
          <span className="truncate">{t.label}</span>
          <span aria-hidden="true" className="text-base text-text-secondary">
            ×
          </span>
        </NextLink>
      ))}
      {activeFilterCount(state) > 0 && (
        <Link href={clearFiltersHref(state)} variant="ghost" size="sm">
          Xóa tất cả bộ lọc
        </Link>
      )}
    </div>
  );
}
