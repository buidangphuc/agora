import {
  type CampaignDraft,
  type CampaignFieldErrors,
  parseCampaignDraft,
} from "@/features/admin/campaignForm";
import type { UpdateCampaignInput } from "@/lib/gateway/campaigns";

export interface CampaignEditValues {
  salePrice: string;
  stockCap: string;
  startsAt: string;
  endsAt: string;
}

export type CampaignEditErrors = Partial<
  Record<keyof CampaignEditValues, string>
>;

export type ParsedEdit =
  | { ok: true; value: UpdateCampaignInput }
  | { ok: false; errors: CampaignEditErrors };

const PLACEHOLDER_LISTING = "listing";

export function parseCampaignEdit(
  values: CampaignEditValues,
  now: Date = new Date(),
): ParsedEdit {
  const draft: CampaignDraft = {
    ...values,
    listingId: PLACEHOLDER_LISTING,
    variantId: "",
  };
  const parsed = parseCampaignDraft(draft, now);
  if (!parsed.ok) {
    const { salePrice, stockCap, startsAt, endsAt } =
      parsed.errors as CampaignFieldErrors;
    return { ok: false, errors: { salePrice, stockCap, startsAt, endsAt } };
  }
  const { salePrice, stockCap, startsAt, endsAt } = parsed.value;
  return { ok: true, value: { salePrice, stockCap, startsAt, endsAt } };
}

const VN_OFFSET_MS = 7 * 60 * 60 * 1000;

export function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return new Date(d.getTime() + VN_OFFSET_MS).toISOString().slice(0, 16);
}
