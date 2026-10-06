import Link from "next/link";

import { Avatar } from "@/components/ui/Avatar";
import { Tag } from "@/components/ui/Tag";

export interface SellerShopInfo {
  sellerId: string;
  /** Already resolved with shopLabel(): the real name, or "Shop #xxxxxx". */
  name: string;
}

/** Sidebar / Drawer shop card: avatar, the real shop name, role tag, public link. */
export function ShopCard({
  shop,
  compact = false,
}: { shop: SellerShopInfo; compact?: boolean }) {
  if (compact) {
    return (
      <div className="flex justify-center">
        <Avatar name={shop.name} size="md" shape="square" />
      </div>
    );
  }
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <Avatar name={shop.name} size="md" shape="square" />
        <div className="min-w-0 flex-1">
          <p
            className="truncate text-sm font-semibold text-text-primary"
            data-testid="seller-shop-name"
          >
            {shop.name}
          </p>
          <div className="mt-1">
            <Tag color="primary">Người bán</Tag>
          </div>
        </div>
      </div>
      {shop.sellerId && (
        <Link
          href={`/shop/${shop.sellerId}`}
          className="block text-xs font-medium text-text-secondary transition hover:text-action-primary"
        >
          Xem gian hàng công khai
        </Link>
      )}
    </div>
  );
}
