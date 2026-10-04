import { redirect } from "next/navigation";

import { ListingForm } from "@/features/listing/ListingForm";
import { LinkButton } from "@/features/seller/LinkButton";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { listCategories } from "@/lib/gateway/listings";
import { hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Đăng sản phẩm mới | Kênh người bán" };

export default async function SellerNewPage() {
  if (!hasScope("listing.write")) redirect("/login");
  const categories = await listCategories();

  return (
    <>
      <SellerPageHeader
        title="Đăng sản phẩm mới"
        description="Điền đầy đủ thông tin để sản phẩm tiếp cận người mua."
        action={<LinkButton href="/seller">Quay lại danh sách</LinkButton>}
      />
      <ListingForm categories={categories} submitLabel="Đăng bán ngay" />
    </>
  );
}
