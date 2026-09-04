"use server";

import { revalidatePath } from "next/cache";

import { type ViewBundle, createBundle } from "@/lib/gateway/listings";
import { requestWalletPayout } from "@/lib/gateway/payment";
import {
  type ViewAdCampaign,
  createAdCampaign,
  subscribe,
} from "@/lib/gateway/promotion";

export async function requestWalletPayoutAction(
  sellerId: string,
  amount: number,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await requestWalletPayout(sellerId, amount);
    revalidatePath("/seller/wallet");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Yêu cầu rút tiền thất bại.",
    };
  }
}

export async function subscribeAction(
  planId: string,
): Promise<{ ok: boolean; message?: string }> {
  const res = await subscribe(planId);
  if (res.ok) revalidatePath("/seller/plans");
  return { ok: res.ok, message: res.message };
}

// ── Listing bundles ─────────────────────────────────────────────────────────

export async function createBundleAction(
  title: string,
  listingIds: string[],
  bundlePrice: number,
): Promise<{ ok: boolean; bundle?: ViewBundle; message?: string }> {
  if (!title.trim()) {
    return { ok: false, message: "Nhập tên combo." };
  }
  if (listingIds.length < 2) {
    return { ok: false, message: "Chọn ít nhất 2 sản phẩm cho combo." };
  }
  if (!(bundlePrice > 0)) {
    return { ok: false, message: "Giá combo phải lớn hơn 0." };
  }
  try {
    const bundle = await createBundle(title, listingIds, bundlePrice);
    revalidatePath("/seller/bundles");
    return { ok: true, bundle };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Tạo combo thất bại.",
    };
  }
}

// ── Sponsored ad campaigns ──────────────────────────────────────────────────

export async function createAdCampaignAction(
  listingId: string,
  budget: number,
  bid: number,
): Promise<{ ok: boolean; campaign?: ViewAdCampaign; message?: string }> {
  if (!listingId) {
    return { ok: false, message: "Chọn sản phẩm cần quảng cáo." };
  }
  if (!(budget > 0) || !(bid > 0)) {
    return { ok: false, message: "Ngân sách và giá thầu phải lớn hơn 0." };
  }
  try {
    const campaign = await createAdCampaign(listingId, budget, bid);
    revalidatePath("/seller/ads");
    return { ok: true, campaign };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Tạo chiến dịch thất bại.",
    };
  }
}
