import { Card } from "@/components/ui/Card";
import type { ViewListing } from "@/lib/gateway/listings";
import { ListingCard } from "./ListingCard";

export function ListingGrid({
  listings,
  empty = "Không tìm thấy sản phẩm nào phù hợp.",
  placementId,
  impressionId,
  modelVersion,
}: {
  listings: ViewListing[];
  empty?: string;
  placementId?: string;
  impressionId?: string;
  modelVersion?: string;
}) {
  if (listings.length === 0) {
    return (
      <Card className="rounded-2xl p-14 text-center border-gray-200/80">
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 text-3xl">
          🔍
        </div>
        <p className="mt-4 text-sm font-semibold text-gray-800">{empty}</p>
        <p className="mt-1 text-xs text-gray-500 max-w-sm mx-auto">
          Hãy thử tìm kiếm với từ khóa khác hoặc điều chỉnh lại bộ lọc giá/danh
          mục để tìm sản phẩm mong muốn.
        </p>
      </Card>
    );
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 sm:gap-3.5">
      {listings.map((l, index) => (
        <ListingCard
          key={l.id}
          listing={l}
          placementId={placementId}
          impressionId={impressionId}
          modelVersion={modelVersion}
          position={index + 1}
        />
      ))}
    </div>
  );
}
