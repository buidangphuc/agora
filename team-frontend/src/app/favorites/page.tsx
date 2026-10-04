import Link from "next/link";
import { redirect } from "next/navigation";

import { Badge } from "@/components/ui/Badge";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Pagination } from "@/components/ui/Pagination";
import { Result } from "@/components/ui/Result";
import { LinkButton } from "@/features/account/LinkButton";
import { parsePage } from "@/features/account/pagination";
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

export const metadata = {
  title: "Sản phẩm yêu thích | Marketplace",
};

// The gateway returns 24 ids per call, behind an opaque cursor.
const FAVORITES_PAGE_SIZE = 24;

interface IdPage {
  ids: string[];
  nextCursor: string;
  total: number;
}

/** Walk the cursor to the requested 1-based page; stops early on the last page. */
async function loadPage(
  load: (cursor: string) => Promise<IdPage>,
  page: number,
): Promise<{ result: IdPage; page: number }> {
  let result = await load("");
  let current = 1;
  while (current < page && result.nextCursor) {
    result = await load(result.nextCursor);
    current += 1;
  }
  return { result, page: current };
}

function hrefFor(collectionId: string, page: number): string {
  const params = new URLSearchParams();
  if (collectionId) params.set("collection", collectionId);
  if (page > 1) params.set("page", String(page));
  const query = params.toString();
  return query ? `/favorites?${query}` : "/favorites";
}

export default async function FavoritesPage({
  searchParams,
}: {
  searchParams?: { collection?: string; page?: string };
}) {
  if (!getPrincipal()) redirect("/login");

  const collections = await listCollections();
  const activeCollectionId = searchParams?.collection ?? "";
  const activeCollection = activeCollectionId
    ? collections.find((c) => c.id === activeCollectionId)
    : undefined;

  if (activeCollectionId && !activeCollection) {
    return (
      <section className="mx-auto max-w-5xl py-2">
        <Result
          status="404"
          title="Không tìm thấy bộ sưu tập"
          subTitle="Bộ sưu tập này không tồn tại hoặc đã bị xóa."
          extra={
            <LinkButton href="/favorites">
              Xem tất cả sản phẩm yêu thích
            </LinkButton>
          }
        />
      </section>
    );
  }

  // When a collection is selected, show its items; otherwise all favorites.
  const { result, page } = await loadPage(
    activeCollectionId
      ? (cursor) => listCollectionItems(activeCollectionId, cursor)
      : (cursor) => listFavoriteIds(cursor),
    parsePage(searchParams?.page),
  );
  const resolved = await Promise.all(result.ids.map((id) => getListing(id)));
  const items = resolved.filter((l): l is ViewListing => l !== null);

  return (
    <section className="mx-auto max-w-5xl space-y-4 py-2">
      <Breadcrumb
        items={[
          { label: "Tài khoản", href: "/account/addresses" },
          { label: "Yêu thích" },
        ]}
      />
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <h1 className="text-2xl font-bold text-text-primary">
            {activeCollection
              ? `Bộ sưu tập: ${activeCollection.name}`
              : "Sản phẩm yêu thích của tôi"}
          </h1>
          <p className="text-sm text-text-secondary">
            {activeCollection
              ? "Các sản phẩm bạn đã thêm vào bộ sưu tập này."
              : "Danh sách các sản phẩm bạn đã lưu để theo dõi giá và khuyến mãi."}
          </p>
        </div>
        <Badge variant="neutral" size="md">
          {result.total} sản phẩm
        </Badge>
      </header>

      <CollectionsManager initialCollections={collections} />

      {activeCollection && (
        <Link
          href="/favorites"
          className="inline-block text-sm font-medium text-action-primary hover:underline"
        >
          ← Xem tất cả sản phẩm yêu thích
        </Link>
      )}

      {items.length === 0 ? (
        <Card>
          <Empty
            description={
              activeCollection
                ? "Bộ sưu tập này chưa có sản phẩm."
                : "Bạn chưa lưu sản phẩm nào. Hãy nhấn vào biểu tượng trái tim trên các sản phẩm bạn thích để lưu lại tại đây nhé."
            }
            action={
              <LinkButton href="/search">Khám phá sản phẩm ngay</LinkButton>
            }
          />
        </Card>
      ) : (
        <ListingGrid listings={items} />
      )}

      <Pagination
        current={page}
        total={result.total}
        pageSize={FAVORITES_PAGE_SIZE}
        hrefFor={(p) => hrefFor(activeCollectionId, p)}
      />
    </section>
  );
}
