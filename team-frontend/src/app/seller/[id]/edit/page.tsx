import Link from "next/link";
import { notFound, redirect } from "next/navigation";

import { ListingForm } from "@/features/listing/ListingForm";
import { getListing, listCategories } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function SellerEditPage({
  params,
}: { params: { id: string } }) {
  if (!hasScope("listing.write")) redirect("/login");

  const [listing, categories] = await Promise.all([
    getListing(params.id),
    listCategories(),
  ]);
  if (!listing) notFound();

  const me = getPrincipal();
  const isOwner = me?.id === listing.sellerId;

  if (!isOwner && !me?.scopes.includes("admin")) {
    return (
      <section className="mx-auto max-w-lg text-center py-12">
        <div className="rounded-xl border border-gray-200 bg-white p-8 shadow-2xs">
          <span className="text-3xl">⚠️</span>
          <h1 className="mt-3 text-base font-bold text-gray-900">
            Không có quyền chỉnh sửa
          </h1>
          <p className="mt-2 text-xs text-gray-500">
            Bạn không phải là người sở hữu sản phẩm này.
          </p>
          <Link
            href="/seller"
            className="mt-5 inline-block rounded-lg bg-brand px-5 py-2 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark"
          >
            Quay lại Kênh người bán
          </Link>
        </div>
      </section>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between border-b border-gray-200 bg-white p-5 rounded-xl shadow-2xs">
        <div>
          <h1 className="text-lg font-bold text-gray-900">
            Chỉnh sửa sản phẩm
          </h1>
          <p className="text-xs text-gray-500 mt-0.5">
            Cập nhật thông tin, giá bán và tồn kho cho mã: #
            {listing.id.slice(0, 8)}
          </p>
        </div>
        <Link
          href="/seller"
          className="text-xs font-medium text-brand hover:underline"
        >
          ← Quay lại danh sách
        </Link>
      </div>

      <ListingForm
        listingId={listing.id}
        categories={categories}
        submitLabel="Lưu thay đổi"
        defaults={{
          id: listing.id,
          title: listing.title,
          description: listing.description,
          price: listing.price,
          currency: listing.currency,
          status: listing.status,
          imageKeys: listing.imageKeys,
          categoryId: listing.categoryId,
          stock: listing.stock,
          variants: listing.variants,
        }}
      />
    </div>
  );
}
