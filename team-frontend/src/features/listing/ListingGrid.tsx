import type { ViewListing } from "@/lib/gateway/listings";

import { ListingCard } from "./ListingCard";

export function ListingGrid({
  listings,
  empty = "Không tìm thấy sản phẩm nào phù hợp.",
}: {
  listings: ViewListing[];
  empty?: string;
}) {
  if (listings.length === 0) {
    return (
      <div className="rounded-xs border border-gray-100 bg-white p-12 text-center shadow-shopee">
        <p className="text-4xl">🔍</p>
        <p className="mt-3 text-sm font-semibold text-gray-700">{empty}</p>
        <p className="mt-1 text-xs text-gray-400">
          Hãy thử tìm kiếm với từ khóa khác hoặc điều chỉnh lại bộ lọc giá/danh
          mục.
        </p>
      </div>
    );
  }
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-2.5">
      {listings.map((l) => (
        <ListingCard key={l.id} listing={l} />
      ))}
    </div>
  );
}
