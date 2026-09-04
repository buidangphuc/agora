import Link from "next/link";
import { redirect } from "next/navigation";

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
    <div className="space-y-4">
      {/* ── Top Header & CTA ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between border-b border-gray-200 bg-white p-5 rounded-xs shadow-2xs">
        <div>
          <h1 className="text-lg font-bold text-gray-900">
            Tất Cả Sản Phẩm Gian Hàng
          </h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Quản lý tồn kho, chỉnh sửa giá bán và theo dõi trạng thái sản phẩm
          </p>
        </div>
        <Link
          href="/seller/new"
          className="inline-flex items-center gap-1.5 rounded-xs bg-brand px-4 py-2 text-xs font-bold text-white shadow-xs hover:bg-brand-dark transition"
        >
          <span>➕</span>
          <span>+ Thêm Sản Phẩm Mới</span>
        </Link>
      </div>

      {/* ── Metric Cards ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs">
          <p className="text-xs font-medium text-gray-500">Tổng sản phẩm</p>
          <p className="mt-1 text-2xl font-bold text-gray-900">{page.total}</p>
        </div>

        <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs">
          <p className="text-xs font-medium text-gray-500">Đang hoạt động</p>
          <p className="mt-1 text-2xl font-bold text-emerald-600">
            {
              page.items.filter((l) => l.status === "published" || !l.status)
                .length
            }
          </p>
        </div>

        <div className="rounded-xs border border-gray-200 bg-white p-4 shadow-2xs">
          <p className="text-xs font-medium text-gray-500">
            Hết hàng / Tạm khóa
          </p>
          <p className="mt-1 text-2xl font-bold text-red-600">0</p>
        </div>
      </div>

      {/* ── Listings Table ── */}
      <div className="overflow-hidden rounded-xs border border-gray-200 bg-white shadow-2xs">
        <div className="border-b border-gray-100 bg-gray-50/70 px-5 py-3 text-xs font-semibold text-gray-700">
          Danh sách sản phẩm của Shop ({page.total})
        </div>

        {error ? (
          <div className="p-6 text-xs text-red-600 bg-red-50">
            Có lỗi xảy ra khi tải danh sách: {error}
          </div>
        ) : page.items.length === 0 ? (
          <div className="p-12 text-center">
            <span className="text-4xl">📦</span>
            <h3 className="mt-3 text-sm font-semibold text-gray-800">
              Shop chưa có sản phẩm nào
            </h3>
            <p className="mt-1 text-xs text-gray-500">
              Hãy đăng sản phẩm đầu tiên để tiếp cận hàng triệu người mua trên
              sàn.
            </p>
            <Link
              href="/seller/new"
              className="mt-4 inline-block rounded-xs bg-brand px-5 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark"
            >
              + Thêm sản phẩm ngay
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-gray-100 bg-gray-50/50 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                <tr>
                  <th className="px-5 py-3">Tên sản phẩm</th>
                  <th className="px-5 py-3">Giá bán</th>
                  <th className="px-5 py-3">Kho hàng</th>
                  <th className="px-5 py-3">Trạng thái</th>
                  <th className="px-5 py-3 text-right">Thao tác</th>
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
                      <td className="px-5 py-3.5 max-w-sm">
                        <div className="flex items-center gap-3">
                          <div className="h-12 w-12 shrink-0 overflow-hidden rounded-xs border border-gray-100 bg-gray-50">
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
                              className="line-clamp-1 font-medium text-gray-900 hover:text-brand"
                            >
                              {l.title}
                            </Link>
                            <p className="text-[10px] text-gray-400 mt-0.5">
                              SKU: #{l.id.slice(0, 8)}
                            </p>
                          </div>
                        </div>
                      </td>

                      {/* Price */}
                      <td className="px-5 py-3.5 font-bold text-brand">
                        {formatPrice(l.price, l.currency)}
                      </td>

                      {/* Stock */}
                      <td className="px-5 py-3.5 font-medium text-gray-800">
                        {l.stock}
                      </td>

                      {/* Status */}
                      <td className="px-5 py-3.5">
                        <span className="inline-block rounded-2xs bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700">
                          ● Đang bán
                        </span>
                      </td>

                      {/* Actions */}
                      <td className="px-5 py-3.5 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Link
                            href={`/listing/${l.id}`}
                            className="rounded-xs px-2.5 py-1 text-gray-600 hover:bg-gray-100 hover:text-brand transition font-medium"
                          >
                            Xem
                          </Link>
                          <Link
                            href={`/seller/${l.id}/edit`}
                            className="rounded-xs px-2.5 py-1 text-brand hover:bg-orange-50 font-semibold transition"
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
      </div>
    </div>
  );
}
