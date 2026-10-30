import { Descriptions } from "@/components/ui/Descriptions";
import { formatPrice } from "@/components/ui/format";
import type { CampaignView } from "@/lib/gateway/campaigns";

import { formatCampaignTime } from "./campaignForm";

export function CampaignDetails({
  campaign,
  title,
  testId,
}: {
  campaign: CampaignView;
  title: string;
  testId: string;
}) {
  return (
    <Descriptions
      data-testid={testId}
      title={title}
      column={2}
      items={[
        { key: "id", label: "Mã chiến dịch", children: campaign.id },
        { key: "listing", label: "Mã tin đăng", children: campaign.listingId },
        {
          key: "variant",
          label: "Mã phân loại",
          children: campaign.variantId || "Tất cả phân loại",
        },
        {
          key: "price",
          label: "Giá flash sale",
          children: formatPrice(campaign.salePrice),
        },
        {
          key: "stock",
          label: "Đã bán / tối đa",
          children: `${campaign.stockSold} / ${campaign.stockCap}`,
        },
        {
          key: "starts",
          label: "Bắt đầu",
          children: formatCampaignTime(campaign.startsAt),
        },
        {
          key: "ends",
          label: "Kết thúc",
          children: formatCampaignTime(campaign.endsAt),
        },
      ]}
    />
  );
}
