import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Alert } from "@/components/ui/Alert";
import { Link } from "@/components/ui/Link";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import {
  type MarketingTab,
  type SearchParams,
  buildListHref,
  parseMarketingParams,
} from "@/features/seller/listParams";
import { FlashSaleTab } from "@/features/seller/marketing/FlashSaleTab";
import { VoucherTab } from "@/features/seller/marketing/VoucherTab";
import { CampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { getListing } from "@/lib/gateway/listings";
import { listMyCampaigns, listMyShopVouchers } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Marketing" };

const BASE = "/seller/marketing";

const TABS: { id: MarketingTab; label: string }[] = [
  { id: "voucher", label: "Voucher của shop" },
  { id: "flashsale", label: "Flash sale" },
];

const STATUS_FILTERS: { id: string; label: string }[] = [
  { id: "all", label: "Tất cả" },
  { id: "scheduled", label: "Sắp diễn ra" },
  { id: "active", label: "Đang diễn ra" },
  { id: "ended", label: "Đã kết thúc" },
  { id: "cancelled", label: "Đã huỷ" },
];

const STATUS_ENUM: Record<string, CampaignStatus> = {
  scheduled: CampaignStatus.SCHEDULED,
  active: CampaignStatus.ACTIVE,
  ended: CampaignStatus.ENDED,
  cancelled: CampaignStatus.CANCELLED,
};

function TabError({ retryHref }: { retryHref: string }) {
  return (
    <div className="space-y-3">
      <Alert
        type="error"
        description="Không tải được danh sách. Vui lòng thử lại."
      />
      <Link href={retryHref} variant="outline" size="xs">
        Thử lại
      </Link>
    </div>
  );
}

export default async function MarketingPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  if (!getPrincipal()) redirectGuestToLogin(BASE);
  if (!hasScope("listing.write")) return null;

  const { tab, cursor, status } = parseMarketingParams(await searchParams);
  const tabHref = (t: MarketingTab) => buildListHref(BASE, { tab: t });
  let body: React.ReactNode;

  if (tab === "voucher") {
    const res = await listMyShopVouchers({ cursor });
    body = !res.ok ? (
      <TabError retryHref={tabHref("voucher")} />
    ) : (
      <>
        <VoucherTab rows={res.items} />
        {res.nextCursor && (
          <Link
            href={buildListHref(BASE, { tab, cursor: res.nextCursor })}
            rel="next"
            variant="outline"
            size="xs"
          >
            Sau
          </Link>
        )}
      </>
    );
  } else {
    const res = await listMyCampaigns({
      cursor,
      statuses: STATUS_ENUM[status] ? [STATUS_ENUM[status]] : [],
    });
    if (!res.ok) {
      body = <TabError retryHref={tabHref("flashsale")} />;
    } else {
      const ids = [...new Set(res.items.map((r) => r.listingId))];
      const titles: Record<string, string> = {};
      await Promise.all(
        ids.map(async (id) => {
          try {
            const l = await getListing(id);
            if (l?.title) titles[id] = l.title;
          } catch {}
        }),
      );
      body = (
        <>
          <nav aria-label="Lọc trạng thái" className="flex flex-wrap gap-2">
            {STATUS_FILTERS.map((f) => (
              <Link
                key={f.id}
                href={buildListHref(BASE, { tab, status: f.id })}
                aria-current={status === f.id ? "true" : undefined}
                variant={status === f.id ? "primary" : "outline"}
                size="xs"
              >
                {f.label}
              </Link>
            ))}
          </nav>
          <FlashSaleTab rows={res.items} titles={titles} />
          {res.nextCursor && (
            <Link
              href={buildListHref(BASE, {
                tab,
                status,
                cursor: res.nextCursor,
              })}
              rel="next"
              variant="outline"
              size="xs"
            >
              Sau
            </Link>
          )}
        </>
      );
    }
  }

  return (
    <div className="space-y-6">
      <SellerPageHeader
        title="Marketing"
        description="Voucher của shop và flash sale của bạn."
      />
      <nav aria-label="Marketing" className="flex flex-wrap gap-2">
        {TABS.map((t) => (
          <Link
            key={t.id}
            href={tabHref(t.id)}
            aria-current={tab === t.id ? "page" : undefined}
            variant={tab === t.id ? "primary" : "outline"}
            size="xs"
          >
            {t.label}
          </Link>
        ))}
      </nav>
      {body}
    </div>
  );
}
