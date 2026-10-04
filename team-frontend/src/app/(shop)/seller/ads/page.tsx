import { redirect } from "next/navigation";

import { AdCampaignForm } from "@/features/seller/AdCampaignForm";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { getAllListings } from "@/features/seller/listingsPage";
import { hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Quảng cáo | Kênh người bán" };

export default async function SellerAdsPage() {
  if (!hasScope("listing.write")) redirect("/login");

  const listings = await getAllListings().catch(() => []);

  return (
    <>
      <SellerPageHeader
        title="Quảng cáo sản phẩm"
        description="Tạo chiến dịch để sản phẩm xuất hiện ở vị trí được tài trợ."
      />
      <AdCampaignForm
        listings={listings.map((l) => ({ id: l.id, title: l.title }))}
      />
    </>
  );
}
