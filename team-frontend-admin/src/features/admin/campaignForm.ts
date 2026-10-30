import type { CreateCampaignInput } from "@/lib/gateway/campaigns";

export interface CampaignDraft {
  listingId: string;
  variantId: string;
  salePrice: string;
  stockCap: string;
  startsAt: string;
  endsAt: string;
}

export type CampaignField = keyof CampaignDraft;

export type CampaignFieldErrors = Partial<Record<CampaignField, string>>;

export type ParsedCampaign =
  | { ok: true; value: CreateCampaignInput }
  | { ok: false; errors: CampaignFieldErrors };

export const EMPTY_DRAFT: CampaignDraft = {
  listingId: "",
  variantId: "",
  salePrice: "",
  stockCap: "",
  startsAt: "",
  endsAt: "",
};

export const CAMPAIGN_ERRORS = {
  forbidden: "Bạn không có quyền thực hiện thao tác này.",
  invalid: "Dữ liệu không hợp lệ; chưa tạo chiến dịch nào.",
  notFound: "Không tìm thấy tin đăng này; chưa tạo chiến dịch nào.",
  unavailable:
    "Hệ thống kiểm toán hoặc dịch vụ khuyến mãi tạm thời không khả dụng. Chưa có chiến dịch nào được tạo; vui lòng thử lại sau.",
  timeout:
    "Chưa rõ chiến dịch đã được tạo hay chưa. Hãy kiểm tra danh sách đang chạy trước khi tạo lại.",
} as const;

const MAX_ID_BYTES = 200;
const TIME_ZONE = "Asia/Ho_Chi_Minh";

export function parseLocalDateTime(raw: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(raw)) return null;
  const d = new Date(`${raw}:00+07:00`);
  return Number.isNaN(d.getTime()) ? null : d;
}

function positiveInt(raw: string): number | null {
  const t = raw.trim();
  if (!/^\d+$/.test(t)) return null;
  const n = Number(t);
  return Number.isSafeInteger(n) && n > 0 ? n : null;
}

function idTooLong(id: string): boolean {
  return new TextEncoder().encode(id).length > MAX_ID_BYTES;
}

export function parseCampaignDraft(
  draft: CampaignDraft,
  now: Date = new Date(),
): ParsedCampaign {
  const errors: CampaignFieldErrors = {};
  const listingId = draft.listingId.trim();
  const variantId = draft.variantId.trim();
  if (!listingId) errors.listingId = "Nhập mã tin đăng.";
  else if (idTooLong(listingId)) errors.listingId = "Mã tin đăng quá dài.";
  if (idTooLong(variantId)) errors.variantId = "Mã phân loại quá dài.";

  const salePrice = positiveInt(draft.salePrice);
  if (salePrice === null) {
    errors.salePrice = "Giá flash sale phải là số nguyên lớn hơn 0.";
  }
  const stockCap = positiveInt(draft.stockCap);
  if (stockCap === null) {
    errors.stockCap = "Số lượng tối đa phải là số nguyên lớn hơn 0.";
  }

  const startsAt = parseLocalDateTime(draft.startsAt);
  const endsAt = parseLocalDateTime(draft.endsAt);
  if (!startsAt) errors.startsAt = "Chọn thời điểm bắt đầu.";
  if (!endsAt) errors.endsAt = "Chọn thời điểm kết thúc.";
  else if (startsAt && endsAt.getTime() <= startsAt.getTime()) {
    errors.endsAt = "Thời điểm kết thúc phải sau thời điểm bắt đầu.";
  } else if (endsAt.getTime() <= now.getTime()) {
    errors.endsAt = "Thời điểm kết thúc phải ở tương lai.";
  }

  if (
    Object.keys(errors).length > 0 ||
    salePrice === null ||
    stockCap === null ||
    !startsAt ||
    !endsAt
  ) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    value: { listingId, variantId, salePrice, stockCap, startsAt, endsAt },
  };
}

export function formatCampaignTime(iso: string): string {
  if (!iso) return "Không giới hạn";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "Không rõ";
  return new Intl.DateTimeFormat("vi-VN", {
    timeZone: TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(d);
}
