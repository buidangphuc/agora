import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Image } from "@/components/ui/Image";
import { Link } from "@/components/ui/Link";
import { PriceTag } from "@/components/ui/PriceTag";
import { Tag } from "@/components/ui/Tag";
import { shopLabel } from "@/lib/gateway/shops";
import { getImageUrl } from "@/lib/media";
import { CartQuantityControl } from "./CartQuantityControl";
import { LineCheckbox, ShopCheckbox } from "./CartSelection";
import type { ShopGroup } from "./groupByShop";

/**
 * One Card per shop: header (real shop display name, "Shop #<6 chars>" only
 * when the name is empty) + one row per item. Server component; the only
 * client islands are the per-row quantity controls.
 */
export function CartGroups({ groups }: { groups: ShopGroup[] }) {
  return (
    <div className="space-y-6">
      {groups.map((group, groupIndex) => (
        <Card key={group.sellerId} data-testid="cart-shop-group">
          <CardHeader>
            <CardTitle>
              <span className="flex min-w-0 items-center gap-3">
                <ShopCheckbox
                  sellerId={group.sellerId}
                  name={shopLabel(group.sellerId, group.sellerDisplayName)}
                />
                <span className="truncate">
                  <Link href={`/shop/${group.sellerId}`} variant="subtle">
                    {shopLabel(group.sellerId, group.sellerDisplayName)}
                  </Link>
                </span>
              </span>
            </CardTitle>
          </CardHeader>
          <ul className="divide-y divide-border-subtle">
            {group.items.map((it) => (
              <li
                key={it.id}
                className="flex flex-col gap-4 px-4 py-5 sm:flex-row sm:items-center sm:gap-6 sm:px-6"
              >
                <div className="flex min-w-0 flex-1 items-start gap-3 sm:items-center sm:gap-4">
                  <LineCheckbox id={it.id} title={it.title} />
                  <div className="w-20 shrink-0 sm:w-24">
                    <Image
                      src={getImageUrl(it.imageUrl)}
                      alt={it.title}
                      aspect="square"
                      loading={groupIndex === 0 ? "eager" : "lazy"}
                    />
                  </div>
                  <div className="min-w-0 flex-1 space-y-2">
                    <span className="line-clamp-2 break-words text-sm font-medium text-text-primary sm:text-base">
                      <Link href={`/listing/${it.listingId}`} variant="subtle">
                        {it.title}
                      </Link>
                    </span>
                    {it.variantName && <Tag>Phân loại: {it.variantName}</Tag>}
                    <div>
                      <PriceTag price={it.unitPrice} size="sm" />
                    </div>
                  </div>
                </div>
                <div className="flex items-center justify-between gap-4 pl-9 sm:shrink-0 sm:justify-end sm:gap-6 sm:pl-0">
                  <CartQuantityControl
                    itemId={it.id}
                    quantity={it.quantity}
                    title={it.title}
                  />
                  <div className="min-w-0 text-right sm:w-32">
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
