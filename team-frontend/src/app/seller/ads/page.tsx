import { redirect } from "next/navigation";

import { AdCampaignForm } from "@/features/seller/AdCampaignForm";
import { type ListingPage, listMyListings } from "@/lib/gateway/listings";
import { hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Quảng cáo | Kênh người bán",
};

export default async function SellerAdsPage() {
  if (!hasScope("listing.write")) redirect("/login");

  let page: ListingPage = { items: [], nextCursor: "", total: 0 };
  try {
    page = await listMyListings();
  } catch {
    // fall through with empty list
  }

  const listings = page.items.map((l) => ({ id: l.id, title: l.title }));

  return (
    <div className="space-y-4">
      <div className="rounded-xs border border-gray-200 bg-white p-5 shadow-2xs">
        <h1 className="text-lg font-bold text-gray-900">
          📣 Quảng cáo sản phẩm
        </h1>
        <p className="mt-0.5 text-xs text-gray-500">
          Tạo chiến dịch quảng cáo để sản phẩm xuất hiện ở vị trí được tài trợ.
        </p>
      </div>

      <AdCampaignForm listings={listings} />
    </div>
  );
}
