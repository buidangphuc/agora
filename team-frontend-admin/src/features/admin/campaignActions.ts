"use server";

import { revalidatePath } from "next/cache";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { type CampaignView, createCampaign } from "@/lib/gateway/campaigns";
import { getPrincipal } from "@/lib/gateway/session";

import {
  CAMPAIGN_ERRORS,
  type CampaignDraft,
  type CampaignFieldErrors,
  parseCampaignDraft,
} from "./campaignForm";

export type CreateCampaignActionResult =
  | { ok: true; campaign: CampaignView }
  | {
      ok: false;
      error: string;
      fieldErrors?: CampaignFieldErrors;
      uncertain?: boolean;
    };

const FORM_PATH = "/admin/campaigns/new";
const LIST_PATH = "/admin/campaigns";

export async function createCampaignAction(
  draft: CampaignDraft,
): Promise<CreateCampaignActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(FORM_PATH);
  if (!principal.scopes.includes("admin")) {
    return { ok: false, error: CAMPAIGN_ERRORS.forbidden };
  }

  const parsed = parseCampaignDraft(draft);
  if (!parsed.ok) {
    return {
      ok: false,
      error: CAMPAIGN_ERRORS.invalid,
      fieldErrors: parsed.errors,
    };
  }

  const res = await createCampaign(parsed.value);
  switch (res.status) {
    case "ok":
      revalidatePath(LIST_PATH);
      return { ok: true, campaign: res.campaign };
    case "unauthenticated":
      return redirectGuestToLogin(FORM_PATH);
    case "forbidden":
      return { ok: false, error: CAMPAIGN_ERRORS.forbidden };
    case "invalid":
      return { ok: false, error: CAMPAIGN_ERRORS.invalid };
    case "not_found":
      return { ok: false, error: CAMPAIGN_ERRORS.notFound };
    case "timeout":
      revalidatePath(LIST_PATH);
      return { ok: false, error: CAMPAIGN_ERRORS.timeout, uncertain: true };
    default:
      return { ok: false, error: CAMPAIGN_ERRORS.unavailable };
  }
}
