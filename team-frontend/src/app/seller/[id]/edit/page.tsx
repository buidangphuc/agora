import { notFound, redirect } from "next/navigation";

import { Result } from "@/components/ui/Result";
import { ListingForm } from "@/features/listing/ListingForm";
import { LinkButton } from "@/features/seller/LinkButton";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { getListing, listCategories } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Chỉnh sửa sản phẩm | Kênh người bán" };

export default async function SellerEditPage({
  params,
}: { params: { id: string } }) {
  if (!hasScope("listing.write")) redirect("/login");

  const [listing, categories] = await Promise.all([
    getListing(params.id),
    listCategories(),
  ]);
  if (!listing) {
    notFound();
    return null;
  }

  const me = getPrincipal();
  const isOwner = me?.id === listing.sellerId;

  if (!isOwner && !me?.scopes.includes("admin")) {
    return (
      <Result
        status="error"
        title="Không có quyền chỉnh sửa"
        subTitle="Bạn không phải là người sở hữu sản phẩm này."
        extra={<LinkButton href="/seller">Quay lại Kênh người bán</LinkButton>}
      />
    );
  }

  return (
    <>
      <SellerPageHeader
        title="Chỉnh sửa sản phẩm"
        description={`Cập nhật thông tin, giá bán và tồn kho cho mã ${listing.id.slice(0, 8)}.`}
        action={<LinkButton href="/seller">Quay lại danh sách</LinkButton>}
      />
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
    </>
  );
}
