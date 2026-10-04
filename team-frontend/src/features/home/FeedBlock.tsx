import Link from "next/link";

import { Alert } from "@/components/ui/Alert";
import { ListingGrid } from "@/features/listing/ListingGrid";
import { loadFeed } from "./data";
import { linkButtonClass } from "./linkButton";

/** "Gợi ý hôm nay": the published-listings feed with its own failure state. */
export async function FeedBlock() {
  const { items, failed } = await loadFeed();

  return (
    <section aria-labelledby="home-feed-title" className="space-y-4">
      <h2
        id="home-feed-title"
        className="flex min-h-16 items-center rounded-2xl border border-border-subtle bg-surface-card px-5 py-4 text-lg font-semibold text-action-primary shadow-preline-card"
      >
        Gợi ý hôm nay
      </h2>

      {failed ? (
        <Alert
          type="error"
          title="Không tải được danh sách sản phẩm"
          description="Vui lòng thử lại sau ít phút."
          action={
            <Link
              href="/"
              className="text-sm font-medium text-action-primary underline"
            >
              Thử lại
            </Link>
          }
        />
      ) : (
        <>
          <ListingGrid
            listings={items}
            empty="Hiện chưa có sản phẩm nào được đăng bán."
          />
          {items.length > 0 && (
            <div className="pt-2 text-center">
              <Link href="/search" className={linkButtonClass("outline")}>
                Xem thêm gợi ý
              </Link>
            </div>
          )}
        </>
      )}
    </section>
  );
}
