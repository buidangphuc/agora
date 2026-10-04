import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import type { ListingPage } from "@/lib/gateway/listings";
import { LinkButton } from "./LinkButton";
import { ProductTable, filterListings } from "./ProductTable";
import { SellerFilterBar } from "./SellerFilterBar";
import { SellerPagination } from "./SellerPagination";
import { type ListParams, buildListHref } from "./listParams";
import { LISTINGS_PAGE_SIZE } from "./listingsPage";

export const LISTING_STATUSES = ["published", "draft", "rejected"] as const;

const STATUS_OPTIONS = [
  { value: "all", label: "Tất cả" },
  { value: "published", label: "Đang bán" },
  { value: "draft", label: "Bản nháp" },
  { value: "rejected", label: "Bị từ chối" },
];

/**
 * Product Table List of /seller: filter bar (URL), Table, Pagination.
 * `listings === null` = the read failed (Alert with retry above an empty table).
 */
export function ProductList({
  listings,
  params,
}: { listings: ListingPage | null; params: ListParams }) {
  const here = buildListHref("/seller", params);
  const hrefFor = (page: number) =>
    buildListHref("/seller", { ...params, page });
  const filtered = listings
    ? filterListings(listings.items, params.q, params.status)
    : [];
  const filtering = params.q !== "" || params.status !== "all";

  let body: React.ReactNode;
  if (listings === null) {
    body = (
      <Alert
        type="error"
        title="Không tải được danh sách sản phẩm"
        description="Có lỗi khi đọc dữ liệu từ máy chủ."
        action={
          <LinkButton href={here} size="sm">
            Thử lại
          </LinkButton>
        }
      />
    );
  } else if (filtered.length === 0) {
    body =
      filtering && listings.total > 0 ? (
        <Empty
          description="Không có kết quả"
          action={
            <LinkButton href="/seller" size="sm">
              Xoá bộ lọc
            </LinkButton>
          }
        />
      ) : (
        <Empty
          description="Shop chưa có sản phẩm"
          action={
            <LinkButton href="/seller/new" size="sm">
              Thêm sản phẩm
            </LinkButton>
          }
        />
      );
  } else {
    body = <ProductTable items={filtered} />;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Sản phẩm{listings ? ` (${listings.total})` : ""}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <SellerFilterBar
          basePath="/seller"
          q={params.q}
          status={params.status}
          statusOptions={STATUS_OPTIONS}
          searchLabel="Tìm sản phẩm"
          placeholder="Tên sản phẩm hoặc mã"
          hint="Tìm kiếm và bộ lọc chỉ áp dụng cho các sản phẩm trong trang này."
        />
        {body}
        {listings && (
          <SellerPagination
            current={params.page}
            total={listings.total}
            pageSize={LISTINGS_PAGE_SIZE}
            hrefFor={hrefFor}
          />
        )}
      </CardContent>
    </Card>
  );
}
