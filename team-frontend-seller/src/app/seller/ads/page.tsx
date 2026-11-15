import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { AdCampaignForm } from "@/features/seller/AdCampaignForm";
import { AdCampaignTable } from "@/features/seller/AdCampaignTable";
import { LinkButton } from "@/features/seller/LinkButton";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { SellerPagination } from "@/features/seller/SellerPagination";
import {
  type SearchParams,
  buildListHref,
  parseListParams,
  slicePage,
} from "@/features/seller/listParams";
import { getAllListings } from "@/features/seller/listingsPage";
import { listMyAdCampaigns } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Quảng cáo" };

const PAGE_SIZE = 20;

export default async function SellerAdsPage({
  searchParams,
}: {
  searchParams?: Promise<SearchParams>;
}) {
  const sp = (await searchParams) ?? {};
  if (!getPrincipal()) redirectGuestToLogin("/seller/ads", sp);
  if (!hasScope("listing.write")) return null;

  const { page } = parseListParams(sp);
  const [listings, result] = await Promise.all([
    getAllListings().catch(() => []),
    listMyAdCampaigns(),
  ]);
  const titles = new Map(listings.map((l) => [l.id, l.title]));
  const here = buildListHref("/seller/ads", { page });

  return (
    <>
      <SellerPageHeader
        title="Quảng cáo sản phẩm"
        description="Tạo chiến dịch để sản phẩm xuất hiện ở vị trí được tài trợ."
      />
      <AdCampaignForm
        listings={listings.map((l) => ({ id: l.id, title: l.title }))}
      />
      <Card>
        <CardHeader>
          <CardTitle>Chiến dịch của bạn</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {result.ok ? (
              <>
                <AdCampaignTable
                  campaigns={slicePage(result.campaigns, page, PAGE_SIZE).map(
                    (c) => ({
                      ...c,
                      listingTitle: titles.get(c.listingId) ?? c.listingId,
                    }),
                  )}
                />
                <SellerPagination
                  current={page}
                  total={result.campaigns.length}
                  pageSize={PAGE_SIZE}
                  hrefFor={(p) => buildListHref("/seller/ads", { page: p })}
                />
              </>
            ) : (
              <Alert
                type="error"
                description="Không tải được chiến dịch"
                action={
                  <LinkButton href={here} size="sm">
                    Thử lại
                  </LinkButton>
                }
              />
            )}
          </div>
        </CardContent>
      </Card>
    </>
  );
}
