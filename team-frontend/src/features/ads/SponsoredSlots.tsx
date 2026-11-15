import { Badge } from "@/components/ui/Badge";
import { ListingCard } from "@/features/listing/ListingCard";
import { LISTING_GRID_CLASS } from "@/features/listing/gridClass";

import { SponsoredClickGuard } from "./SponsoredClickGuard";
import type { SponsoredItem } from "./data";

export function SponsoredSlots({ items }: { items: SponsoredItem[] }) {
  if (items.length === 0) return null;
  return (
    <SponsoredClickGuard>
      <div className={LISTING_GRID_CLASS}>
        {items.map((item) => (
          <div
            key={item.listing.id}
            data-testid="sponsored-slot"
            data-impression-token={item.impressionToken}
            className="relative"
          >
            <Badge variant="neutral" size="xs">
              Tài trợ
            </Badge>
            <ListingCard listing={item.listing} />
          </div>
        ))}
      </div>
    </SponsoredClickGuard>
  );
}
