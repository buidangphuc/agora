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
import { getListing } from "@/lib/gateway/listings";
import { listMyAdCampaigns } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Quảng cáo" };

const PAGE_SIZE = 20;

async function listingTitles(ids: string[]): Promise<Map<string, string>> {
  const unique = [...new Set(ids)];
  const found = await Promise.all(
    unique.map((id) => getListing(id).catch(() => null)),
  );
  const titles = new Map<string, string>();
  unique.forEach((id, i) => {
    const title = found[i]?.title;
    if (title) titles.set(id, title);
  });
  return titles;
}

export default async function SellerAdsPage({
  searchParams,
}: {
  searchParams?: Promise<SearchParams>;
}) {
  const sp = (await searchParams) ?? {};
  if (!getPrincipal()) redirectGuestToLogin("/seller/ads", sp);
  if (!hasScope("listing.write")) return null;

  const { page } = parseListParams(sp);
  const result = await listMyAdCampaigns();
  const visible = result.ok ? slicePage(result.campaigns, page, PAGE_SIZE) : [];
  const titles = await listingTitles(visible.map((c) => c.listingId));
  const here = buildListHref("/seller/ads", { page });

  return (
    <>
      <SellerPageHeader
        title="Quảng cáo sản phẩm"
        description="Tạo chiến dịch để sản phẩm xuất hiện ở vị trí được tài trợ."
      />
      <AdCampaignForm />
      <Card>
        <CardHeader>
          <CardTitle>Chiến dịch của bạn</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {result.ok ? (
              <>
                <AdCampaignTable
                  campaigns={visible.map((c) => ({
                    ...c,
                    listingTitle: titles.get(c.listingId) ?? c.listingId,
                  }))}
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
