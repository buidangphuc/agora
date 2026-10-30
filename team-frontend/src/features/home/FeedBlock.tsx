import { Alert } from "@/components/ui/Alert";
import { Link } from "@/components/ui/Link";
import { ListingGrid } from "@/features/listing/ListingGrid";
import { SectionHeading } from "./SectionHeading";
import { loadFeed } from "./data";

/** "Gợi ý hôm nay": the published-listings feed with its own failure state. */
export async function FeedBlock() {
  const { items, failed } = await loadFeed();

  return (
    <section aria-labelledby="home-feed-title" className="space-y-5">
      <SectionHeading
        id="home-feed-title"
        title="Gợi ý hôm nay"
        description="Sản phẩm mới đăng bán từ các shop."
      />

      {failed ? (
        <Alert
          type="error"
          title="Không tải được danh sách sản phẩm"
          description="Vui lòng thử lại sau ít phút."
          action={
            <Link href="/" variant="secondary" size="sm">
              Thử lại
            </Link>
          }
        />
      ) : (
        <>
          <ListingGrid
            listings={items}
            placementId="home_latest"
            empty="Hiện chưa có sản phẩm nào được đăng bán."
          />
          {items.length > 0 && (
            <div className="flex justify-center pt-2">
              <Link
                href="/search"
                variant="secondary"
                size="md"
                fullWidth="mobile"
              >
                Xem thêm gợi ý
              </Link>
            </div>
          )}
        </>
      )}
    </section>
  );
}
