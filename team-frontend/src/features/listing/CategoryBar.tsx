import Link from "next/link";

import type { ViewCategory } from "@/lib/gateway/listings";

export function CategoryBar({
  categories,
  selectedId,
  baseUrl = "/search",
}: {
  categories: ViewCategory[];
  selectedId?: string;
  baseUrl?: string;
}) {
  if (!categories || categories.length === 0) return null;

  return (
    <div className="mb-6 flex gap-2 overflow-x-auto pb-2 scrollbar-none">
      <Link
        href={baseUrl}
        className={`inline-flex shrink-0 items-center rounded-full px-4 py-1.5 text-sm font-medium transition ${
          !selectedId
            ? "bg-brand text-white shadow-sm"
            : "bg-gray-100 text-gray-700 hover:bg-gray-200"
        }`}
      >
        Tất cả
      </Link>
      {categories.map((c) => {
        const isSelected = selectedId === c.id;
        const href = `${baseUrl}?category=${encodeURIComponent(c.id)}`;
        return (
          <Link
            key={c.id}
            href={href}
            className={`inline-flex shrink-0 items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-medium transition ${
              isSelected
                ? "bg-brand text-white shadow-sm"
                : "bg-gray-100 text-gray-700 hover:bg-gray-200"
            }`}
          >
            {c.iconUrl && <span>{c.iconUrl}</span>}
            <span>{c.name}</span>
          </Link>
        );
      })}
    </div>
  );
}
