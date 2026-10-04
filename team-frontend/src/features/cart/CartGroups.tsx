import Link from "next/link";

import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Image } from "@/components/ui/Image";
import { PriceTag } from "@/components/ui/PriceTag";
import { Tag } from "@/components/ui/Tag";
import { shopLabel } from "@/lib/gateway/shops";
import { getImageUrl } from "@/lib/media";
import { CartQuantityControl } from "./CartQuantityControl";
import type { ShopGroup } from "./groupByShop";

/**
 * One Card per shop: header (real shop display name, "Shop #<6 chars>" only
 * when the name is empty) + one row per item. Server component; the only
 * client islands are the per-row quantity controls.
 */
export function CartGroups({ groups }: { groups: ShopGroup[] }) {
  return (
    <div className="space-y-4">
      {groups.map((group, groupIndex) => (
        <Card key={group.sellerId} data-testid="cart-shop-group">
          <CardHeader>
            <CardTitle>
              <Link
                href={`/shop/${group.sellerId}`}
                className="hover:text-action-primary"
              >
                {shopLabel(group.sellerId, group.sellerDisplayName)}
              </Link>
            </CardTitle>
          </CardHeader>
          <ul className="divide-y divide-border-subtle">
            {group.items.map((it) => (
              <li
                key={it.id}
                className="flex flex-wrap gap-3 p-4 sm:flex-nowrap sm:items-center"
              >
                <div className="w-20 shrink-0">
                  <Image
                    src={getImageUrl(it.imageUrl)}
                    alt={it.title}
                    aspect="square"
                    loading={groupIndex === 0 ? "eager" : "lazy"}
                  />
                </div>
                <div className="min-w-0 flex-1 space-y-1.5">
                  <Link
                    href={`/listing/${it.listingId}`}
                    className="line-clamp-2 text-sm font-medium text-text-primary hover:text-action-primary"
                  >
                    {it.title}
                  </Link>
                  {it.variantName && <Tag>Phân loại: {it.variantName}</Tag>}
                  <div>
                    <PriceTag price={it.unitPrice} size="sm" />
                  </div>
                </div>
                <div className="flex w-full items-center justify-between gap-4 sm:w-auto sm:justify-end sm:gap-6">
                  <CartQuantityControl
                    itemId={it.id}
                    quantity={it.quantity}
                    title={it.title}
                  />
                  <div className="w-28 text-right">
                    <PriceTag price={it.unitPrice * it.quantity} size="md" />
                  </div>
                </div>
              </li>
            ))}
          </ul>
        </Card>
      ))}
    </div>
  );
}
