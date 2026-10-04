import Link from "next/link";

import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { focusRing } from "@/components/ui/focus";
import { range } from "@/components/ui/range";
import { searchListings } from "@/lib/gateway/search";

const FEED_LIMIT = 24;
const ROW_HEIGHT = "h-12";

/** Placeholder with the same footprint as the feed rows (shown behind Suspense). */
export function FollowedFeedSkeleton() {
  return (
    <Card aria-busy="true" data-testid="followed-feed-skeleton">
      <ul className="divide-y divide-border-subtle">
        {range(6).map((i) => (
          <li key={i} className={`flex items-center px-5 ${ROW_HEIGHT}`}>
            <div className="h-4 w-2/3 animate-pulse rounded-xs bg-neutral-200" />
          </li>
        ))}
      </ul>
    </Card>
  );
}

/**
 * Published listings from the followed sellers. team-engagement's
 * ListFollowedListings depends on a listing-event consumer that is not wired
 * yet, so the feed is resolved through the search service (filter by seller),
 * which is live.
 */
export async function FollowedFeed({ sellerIds }: { sellerIds: string[] }) {
  const perSeller = await Promise.all(
    sellerIds.map((sid) =>
      searchListings("", { sellerId: sid })
        .then((r) => r.items)
        .catch(() => []),
    ),
  );
  const seen = new Set<string>();
  const listings = perSeller
    .flat()
    .filter((l) => {
      if (seen.has(l.id)) return false;
      seen.add(l.id);
      return true;
    })
    .slice(0, FEED_LIMIT)
    .map((l) => ({ id: l.id, title: l.title }));

  if (listings.length === 0) {
    return (
      <Card>
        <Empty description="Bạn chưa theo dõi sản phẩm nào." />
      </Card>
    );
  }

  return (
    <Card>
      <ul className="divide-y divide-border-subtle">
        {listings.map((l) => (
          <li key={l.id}>
            <Link
              href={`/listing/${l.id}`}
              className={`flex min-h-12 items-center px-5 py-3 text-sm font-medium text-text-primary transition duration-150 hover:bg-surface-muted ${focusRing}`}
            >
              {l.title}
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  );
}
