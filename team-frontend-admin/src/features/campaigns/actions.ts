"use server";

import { revalidatePath } from "next/cache";

import {
  type CampaignDraft,
  type CampaignFieldErrors,
  parseCampaignDraft,
} from "@/features/admin/campaignForm";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  type CampaignView,
  cancelCampaign,
  createCampaign,
  updateCampaign,
} from "@/lib/gateway/campaigns";
import { isPromotionAdmin } from "@/lib/gateway/scopes";
import { getPrincipal } from "@/lib/gateway/session";

import {
  type CampaignEditErrors,
  type CampaignEditValues,
  parseCampaignEdit,
} from "./campaignForm";
import { campaignErrorMessage } from "./campaignMessages";

export type CampaignActionResult =
  | { ok: true; campaign: CampaignView }
  | {
      ok: false;
      error: string;
      fieldErrors?: CampaignFieldErrors | CampaignEditErrors;
      uncertain?: boolean;
    };

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

type Outcome = { status: "ok"; campaign: CampaignView } | { status: string };

function settle(res: Outcome, guestPath: string): CampaignActionResult {
  if (res.status === "ok" && "campaign" in res) {
    revalidatePath(LIST_PATH);
    return { ok: true, campaign: res.campaign };
  }
  if (res.status === "unauthenticated") return redirectGuestToLogin(guestPath);
  if (res.status === "timeout") {
    revalidatePath(LIST_PATH);
    return {
      ok: false,
      error: campaignErrorMessage("timeout"),
      uncertain: true,
    };
  }
  return { ok: false, error: campaignErrorMessage(res.status) };
}

export async function createCampaignAction(
  draft: CampaignDraft,
  idempotencyKey: string,
): Promise<CreateCampaignActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(FORM_PATH);
  if (!isPromotionAdmin(principal)) {
    return { ok: false, error: campaignErrorMessage("forbidden") };
  }
  const parsed = parseCampaignDraft(draft);
  if (!parsed.ok) {
    return {
      ok: false,
      error: campaignErrorMessage("invalid"),
      fieldErrors: parsed.errors,
    };
  }
  return settle(
    await createCampaign(parsed.value, idempotencyKey),
    FORM_PATH,
  ) as CreateCampaignActionResult;
}

export async function cancelCampaignAction(
  campaignId: string,
): Promise<CampaignActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(LIST_PATH);
  if (!isPromotionAdmin(principal)) {
    return { ok: false, error: campaignErrorMessage("forbidden") };
  }
  return settle(await cancelCampaign(campaignId), LIST_PATH);
}

export async function updateCampaignAction(
  campaignId: string,
  values: CampaignEditValues,
): Promise<CampaignActionResult> {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLogin(LIST_PATH);
  if (!isPromotionAdmin(principal)) {
    return { ok: false, error: campaignErrorMessage("forbidden") };
  }
  const parsed = parseCampaignEdit(values);
  if (!parsed.ok) {
    return {
      ok: false,
      error: campaignErrorMessage("invalid"),
      fieldErrors: parsed.errors,
    };
  }
  return settle(await updateCampaign(campaignId, parsed.value), LIST_PATH);
}
