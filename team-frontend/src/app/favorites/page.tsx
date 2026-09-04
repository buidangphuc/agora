import Link from "next/link";
import { redirect } from "next/navigation";

import { CollectionsManager } from "@/features/engagement/CollectionsManager";
import { ListingGrid } from "@/features/listing/ListingGrid";
import {
  listCollectionItems,
  listCollections,
  listFavoriteIds,
} from "@/lib/gateway/engagement";
import { type ViewListing, getListing } from "@/lib/gateway/listings";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function FavoritesPage({
  searchParams,
}: {
  searchParams?: { collection?: string };
}) {
  if (!getPrincipal()) redirect("/login");

  const collections = await listCollections();
  const activeCollectionId = searchParams?.collection ?? "";
  const activeCollection = activeCollectionId
    ? collections.find((c) => c.id === activeCollectionId)
    : undefined;

  // When a collection is selected, show its items; otherwise all favorites.
  const { ids, total } = activeCollectionId
    ? await listCollectionItems(activeCollectionId)
    : await listFavoriteIds();

  const resolved = await Promise.all(ids.map((id) => getListing(id)));
  const items = resolved.filter((l): l is ViewListing => l !== null);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between border-b bg-white p-5 rounded-xl shadow-2xs">
        <div>
          <h1 className="text-lg font-bold text-gray-900 flex items-center gap-2">
            <span>♥</span>
            <span>
              {activeCollection
                ? `Bộ sưu tập: ${activeCollection.name}`
                : "Sản phẩm yêu thích của tôi"}
            </span>
          </h1>
          <p className="text-xs text-gray-500 mt-0.5">
            {activeCollection
              ? "Các sản phẩm bạn đã thêm vào bộ sưu tập này"
              : "Danh sách các sản phẩm bạn đã lưu để theo dõi giá và khuyến mãi"}
          </p>
        </div>

        <span className="rounded bg-brand/10 px-3 py-1 text-xs font-bold text-brand">
          {total} sản phẩm
        </span>
      </div>

      {/* Collections manager */}
      <CollectionsManager initialCollections={collections} />

      {activeCollection && (
        <Link
          href="/favorites"
          className="inline-block text-xs font-medium text-brand hover:underline"
        >
          ← Xem tất cả sản phẩm yêu thích
        </Link>
      )}

      {/* Grid */}
      {items.length === 0 ? (
        <div className="rounded-xl border border-gray-100 bg-white p-12 text-center shadow-2xs">
          <span className="text-4xl">♥</span>
          <h2 className="mt-3 text-base font-bold text-gray-800">
            {activeCollection
              ? "Bộ sưu tập này chưa có sản phẩm"
              : "Bạn chưa lưu sản phẩm nào"}
          </h2>
          <p className="mt-1 text-xs text-gray-500">
            Hãy nhấn vào biểu tượng trái tim trên các sản phẩm bạn thích để lưu
            lại tại đây nhé.
          </p>
          <Link
            href="/search"
            className="mt-5 inline-block rounded-lg bg-brand px-6 py-2.5 text-xs font-semibold text-white shadow-xs hover:bg-brand-dark transition"
          >
            Khám phá sản phẩm ngay
          </Link>
        </div>
      ) : (
        <ListingGrid listings={items} />
      )}
    </div>
  );
}
