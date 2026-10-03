import Link from "next/link";
import { redirect } from "next/navigation";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { formatPrice } from "@/components/ui/format";
import { DeleteListingButton } from "@/features/listing/DeleteListingButton";
import { type ListingPage, listMyListings } from "@/lib/gateway/listings";
import { hasScope } from "@/lib/gateway/session";
import { getImageUrl } from "@/lib/media";

export const dynamic = "force-dynamic";

export default async function SellerPage() {
  if (!hasScope("listing.write")) redirect("/login");

  let page: ListingPage = { items: [], nextCursor: "", total: 0 };
  let error: string | null = null;
  try {
    page = await listMyListings();
  } catch (err) {
    error = String(err);
  }

  return (
    <div className="space-y-5">
      {/* ── Top Header & CTA ── */}
      <Card className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between p-6 rounded-2xl border-gray-200/80 shadow-preline-card">
        <div>
          <h1 className="text-lg font-bold text-gray-900 tracking-tight">
            Tất Cả Sản Phẩm Gian Hàng
          </h1>
          <p className="text-xs text-gray-500 mt-1">
            Quản lý tồn kho, chỉnh sửa giá bán và theo dõi trạng thái sản phẩm
          </p>
        </div>
        <Link href="/seller/new">
          <Button
            variant="primary"
            size="md"
            className="font-semibold shadow-sm"
          >
            <span>+ Thêm Sản Phẩm Mới</span>
          </Button>
        </Link>
      </Card>

      {/* ── Metric Cards ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Card className="p-5 rounded-2xl border-gray-200/80 shadow-preline-card">
          <p className="text-xs font-medium text-gray-500">Tổng sản phẩm</p>
          <p className="mt-2 text-2xl font-bold text-gray-900">{page.total}</p>
        </Card>

        <Card className="p-5 rounded-2xl border-gray-200/80 shadow-preline-card">
          <p className="text-xs font-medium text-gray-500">Đang hoạt động</p>
          <p className="mt-2 text-2xl font-bold text-emerald-600">
            {
              page.items.filter((l) => l.status === "published" || !l.status)
                .length
            }
          </p>
        </Card>

        <Card className="p-5 rounded-2xl border-gray-200/80 shadow-preline-card">
          <p className="text-xs font-medium text-gray-500">
            Hết hàng / Tạm khóa
          </p>
          <p className="mt-2 text-2xl font-bold text-rose-600">0</p>
        </Card>
      </div>

      {/* ── Listings Table ── */}
      <Card className="overflow-hidden rounded-2xl border-gray-200/80 shadow-preline-card">
        <div className="border-b border-gray-100 bg-gray-50/70 px-6 py-4 text-xs font-semibold text-gray-700">
          Danh sách sản phẩm của Shop ({page.total})
        </div>

        {error ? (
          <div className="p-6 text-xs text-red-600 bg-red-50">
            Có lỗi xảy ra khi tải danh sách: {error}
          </div>
        ) : page.items.length === 0 ? (
          <div className="p-14 text-center">
            <span className="text-5xl">📦</span>
            <h3 className="mt-4 text-sm font-semibold text-gray-900">
              Shop chưa có sản phẩm nào
            </h3>
            <p className="mt-1 text-xs text-gray-500 max-w-sm mx-auto">
              Hãy đăng sản phẩm đầu tiên để tiếp cận hàng triệu người mua trên
              sàn.
            </p>
            <div className="mt-5">
              <Link href="/seller/new">
                <Button variant="primary" size="md">
                  + Thêm sản phẩm ngay
                </Button>
              </Link>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-gray-100 bg-gray-50/50 text-xs font-semibold uppercase tracking-wider text-gray-500">
                <tr>
                  <th className="px-6 py-3.5">Tên sản phẩm</th>
                  <th className="px-6 py-3.5">Giá bán</th>
                  <th className="px-6 py-3.5">Kho hàng</th>
                  <th className="px-6 py-3.5">Trạng thái</th>
                  <th className="px-6 py-3.5 text-right">Thao tác</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {page.items.map((l) => {
                  const imageSrc =
                    l.imageKeys && l.imageKeys.length > 0
                      ? getImageUrl(l.imageKeys[0])
                      : l.imageUrl;

                  return (
                    <tr key={l.id} className="hover:bg-gray-50/60 transition">
                      {/* Product details */}
                      <td className="px-6 py-4 max-w-sm">
                        <div className="flex items-center gap-3">
                          <div className="h-12 w-12 shrink-0 overflow-hidden rounded-lg border border-gray-100 bg-gray-50">
                            {imageSrc ? (
                              // eslint-disable-next-line @next/next/no-img-element
                              <img
                                src={imageSrc}
                                alt={l.title}
                                className="h-full w-full object-cover"
                              />
                            ) : (
                              <div className="grid h-full w-full place-items-center text-xs text-gray-400">
                                📦
                              </div>
                            )}
                          </div>
                          <div className="min-w-0">
                            <Link
                              href={`/listing/${l.id}`}
                              className="line-clamp-1 font-medium text-gray-900 hover:text-primary-600 transition"
                            >
                              {l.title}
                            </Link>
                            <p className="text-xs text-gray-400 mt-0.5">
                              SKU: #{l.id.slice(0, 8)}
                            </p>
                          </div>
                        </div>
                      </td>

                      {/* Price */}
                      <td className="px-6 py-4 font-bold text-primary-600">
                        {formatPrice(l.price, l.currency)}
                      </td>

                      {/* Stock */}
                      <td className="px-6 py-4 font-medium text-gray-800">
                        {l.stock}
                      </td>

                      {/* Status */}
                      <td className="px-6 py-4">
                        <Badge variant="success" size="xs">
                          ● Đang bán
                        </Badge>
                      </td>

                      {/* Actions */}
                      <td className="px-6 py-4 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Link
                            href={`/listing/${l.id}`}
                            className="rounded-md px-2.5 py-1 text-gray-600 hover:bg-gray-100 hover:text-gray-900 transition font-medium"
                          >
                            Xem
                          </Link>
                          <Link
                            href={`/seller/${l.id}/edit`}
                            className="rounded-md px-2.5 py-1 text-primary-600 hover:bg-primary-50 font-semibold transition"
                          >
                            Sửa
                          </Link>
                          <DeleteListingButton id={l.id} />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
