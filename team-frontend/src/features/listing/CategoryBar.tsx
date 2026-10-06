import Link from "next/link";

import { focusRing } from "@/components/ui/focus";
import type { ViewCategory } from "@/lib/gateway/listings";

export type CategoryBarVariant = "grid" | "pills";

function categoryHref(baseUrl: string, id: string): string {
  return `${baseUrl}?category=${encodeURIComponent(id)}`;
}

/**
 * Category navigation as plain links to `<baseUrl>?category=<id>` (no client
 * code). `grid` is the home tile grid; `pills` is the horizontally scrollable
 * strip above results. The selected category carries `aria-current`.
 */
export function CategoryBar({
  categories,
  selectedId,
  baseUrl = "/search",
  variant = "pills",
}: {
  categories: ViewCategory[];
  selectedId?: string;
  baseUrl?: string;
  variant?: CategoryBarVariant;
}) {
  if (!categories || categories.length === 0) return null;

  if (variant === "grid") {
    return (
      <nav aria-label="Danh mục sản phẩm">
        <ul className="grid grid-cols-2 divide-x divide-y divide-border-subtle sm:grid-cols-5 md:grid-cols-10">
          {categories.map((c) => {
            const isSelected = selectedId === c.id;
            return (
              <li key={c.id}>
                <Link
                  href={categoryHref(baseUrl, c.id)}
                  aria-current={isSelected ? "true" : undefined}
                  className={`group flex min-h-24 flex-col items-center justify-center gap-2 p-3 text-center transition duration-150 hover:bg-surface-muted ${focusRing} ${
                    isSelected ? "bg-primary-50" : "bg-surface-card"
                  }`}
                >
                  <span
                    aria-hidden="true"
                    className="flex h-12 w-12 items-center justify-center text-2xl"
                  >
                    {c.iconUrl || "•"}
                  </span>
                  <span
                    className={`line-clamp-2 text-xs font-medium leading-4 group-hover:text-action-primary ${
                      isSelected ? "text-action-primary" : "text-text-primary"
                    }`}
                  >
                    {c.name}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>
    );
  }

  const pill = `inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-full px-4 text-sm font-medium transition duration-150 ${focusRing}`;
  const on = "bg-action-primary text-text-inverse shadow-sm";
  const off = "bg-surface-page text-text-primary hover:bg-border-subtle";

  return (
    <nav aria-label="Danh mục sản phẩm" className="mb-6">
      <ul className="flex gap-2 overflow-x-auto pb-2 scrollbar-none">
        <li className="shrink-0">
          <Link
            href={baseUrl}
            aria-current={selectedId ? undefined : "true"}
            className={`${pill} ${selectedId ? off : on}`}
          >
            Tất cả
          </Link>
        </li>
        {categories.map((c) => {
          const isSelected = selectedId === c.id;
          return (
            <li key={c.id} className="shrink-0">
              <Link
                href={categoryHref(baseUrl, c.id)}
                aria-current={isSelected ? "true" : undefined}
                className={`${pill} ${isSelected ? on : off}`}
              >
                {c.iconUrl && <span aria-hidden="true">{c.iconUrl}</span>}
                <span>{c.name}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
