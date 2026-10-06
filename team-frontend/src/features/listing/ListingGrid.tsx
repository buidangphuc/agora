import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import type { ViewListing } from "@/lib/gateway/listings";
import { ListingCard } from "./ListingCard";
import { LISTING_GRID_CLASS } from "./gridClass";

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
      <Card>
        <Empty
          description={
            <>
              <span className="block font-semibold text-text-primary">
                {empty}
              </span>
              <span className="mt-1 block text-xs">
                Hãy thử từ khóa khác hoặc thay đổi bộ lọc để tìm sản phẩm bạn
                muốn.
              </span>
            </>
          }
        />
      </Card>
    );
  }

  return (
    <div className={LISTING_GRID_CLASS}>
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
