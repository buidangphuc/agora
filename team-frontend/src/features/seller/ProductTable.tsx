import { Image } from "@/components/ui/Image";
import { Link } from "@/components/ui/Link";
import { PriceTag } from "@/components/ui/PriceTag";
import { Table, type TableColumn } from "@/components/ui/Table";
import { DeleteListingModal } from "@/features/listing/DeleteListingModal";
import type { ViewListing } from "@/lib/gateway/listings";
import { ListingStatusTag } from "./status";

/** Thumbnails inside the first screen load eagerly; the rest are lazy. */
export const EAGER_ROWS = 5;

/** Product Table List: thumbnail, title link, price, stock, status, actions. */
export function ProductTable({ items }: { items: ViewListing[] }) {
  const columns: TableColumn<ViewListing>[] = [
    {
      key: "product",
      title: "Sản phẩm",
      sticky: true,
      render: (l, i) => (
        <div className="flex min-w-64 items-center gap-3">
          <div className="w-12 shrink-0">
            <Image
              src={l.imageUrl ?? ""}
              alt={l.title}
              aspect="square"
              loading={i < EAGER_ROWS ? "eager" : "lazy"}
              className="rounded-lg"
            />
          </div>
          <div className="min-w-0">
            {/* Plain link: same target as before (tracking untouched). */}
            <span className="line-clamp-1 font-medium text-text-primary">
              <Link href={`/listing/${l.id}`} variant="subtle">
                {l.title}
              </Link>
            </span>
            <p className="mt-0.5 text-xs text-text-disabled">
              Mã: {l.id.slice(0, 8)}
            </p>
          </div>
        </div>
      ),
    },
    {
      key: "price",
      title: "Giá bán",
      render: (l) => <PriceTag price={l.price} size="md" />,
    },
    { key: "stock", title: "Kho", render: (l) => l.stock },
    {
      key: "status",
      title: "Trạng thái",
      render: (l) => <ListingStatusTag status={l.status} />,
    },
    {
      key: "actions",
      title: "Thao tác",
      align: "right",
      render: (l) => (
        <div className="flex items-center justify-end gap-1">
          <Link href={`/listing/${l.id}`} variant="ghost" size="xs">
            Xem
          </Link>
          <Link href={`/seller/${l.id}/edit`} variant="ghost" size="xs">
            Sửa
          </Link>
          <DeleteListingModal id={l.id} title={l.title} />
        </div>
      ),
    },
  ];

  return (
    <Table
      caption="Danh sách sản phẩm"
      columns={columns}
      dataSource={items}
      rowKey="id"
    />
  );
}

/** Page-local text / status filter (the gateway only offers a cursor). */
export function filterListings(
  items: ViewListing[],
  q: string,
  status: string,
): ViewListing[] {
  const needle = q.trim().toLowerCase();
  return items.filter((l) => {
    if (status !== "all" && l.status !== status) return false;
    if (!needle) return true;
    return (
      l.title.toLowerCase().includes(needle) ||
      l.id.toLowerCase().startsWith(needle)
    );
  });
}
