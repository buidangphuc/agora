import { DiscountType } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { CampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { MyCampaignRow, MyVoucherRow } from "@/lib/gateway/promotion";

export function voucherRow(over: Partial<MyVoucherRow> = {}): MyVoucherRow {
  return {
    id: "v1",
    code: "GIAM10",
    discountType: DiscountType.PERCENT,
    discountValue: 10,
    minSpend: 0,
    maxDiscount: 0,
    quota: 100,
    used: 3,
    startsAt: "2030-01-01T00:00:00Z",
    endsAt: "2030-02-01T00:00:00Z",
    active: true,
    state: "active",
    stateText: "Đang chạy",
    ...over,
  };
}

const STATUS_TEXT: Record<number, string> = {
  [CampaignStatus.SCHEDULED]: "Sắp diễn ra",
  [CampaignStatus.ACTIVE]: "Đang diễn ra",
  [CampaignStatus.ENDED]: "Đã kết thúc",
  [CampaignStatus.CANCELLED]: "Đã huỷ",
};

export function campaignRow(
  over: Partial<MyCampaignRow> & { status?: CampaignStatus } = {},
): MyCampaignRow {
  const status = over.status ?? CampaignStatus.SCHEDULED;
  const cap = over.stockCap ?? 10;
  const sold = over.stockSold ?? 0;
  return {
    id: "c1",
    listingId: "lst-1",
    variantId: "",
    salePrice: 50000,
    stockCap: cap,
    stockSold: sold,
    soldText: `${sold}/${cap}`,
    startsAt: "2030-01-01T00:00:00Z",
    endsAt: "2030-02-01T00:00:00Z",
    statusText: STATUS_TEXT[status],
    ...over,
    status,
  };
}
