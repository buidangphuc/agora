import { Image } from "@/components/ui/Image";
import { Link } from "@/components/ui/Link";
import { PriceTag } from "@/components/ui/PriceTag";
import { Table, type TableColumn } from "@/components/ui/Table";
import { DeleteListingModal } from "@/features/listing/DeleteListingModal";
import { buyerAppUrl } from "@/lib/apps";
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
        <div className="flex min-w-56 items-center gap-4 sm:min-w-64">
          <div className="w-14 shrink-0">
            <Image
              src={l.imageUrl ?? ""}
              alt={l.title}
              aspect="square"
              loading={i < EAGER_ROWS ? "eager" : "lazy"}
              radius="xl"
            />
          </div>
          <div className="min-w-0 space-y-1">
            <span className="line-clamp-2 font-semibold leading-snug text-text-primary">
              <Link href={buyerAppUrl(`/listing/${l.id}`)} variant="subtle">
                {l.title}
              </Link>
            </span>
            <p className="text-xs text-text-secondary">
              Mã: {l.id.slice(0, 8)}
              <span className="sm:hidden"> · Kho: {l.stock}</span>
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
    {
      key: "stock",
      title: "Kho",
      hideBelow: "sm",
      render: (l) => (
        <span className="tabular-nums text-text-secondary">{l.stock}</span>
      ),
    },
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
        <div className="flex items-center justify-end gap-2">
          <Link
            href={buyerAppUrl(`/listing/${l.id}`)}
            variant="ghost"
            size="xs"
          >
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
