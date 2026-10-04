"use server";

import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type ViewBundle,
  createBundle,
  getStorefront,
  upsertStorefront,
} from "@/lib/gateway/listings";
import { requestWalletPayout } from "@/lib/gateway/payment";
import {
  type ViewAdCampaign,
  createAdCampaign,
  subscribe,
} from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import { validateShopName } from "./shopName";

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

// ── Wallet ──────────────────────────────────────────────────────────────────

export async function requestWalletPayoutAction(
  sellerId: string,
  amount: number,
): Promise<ActionResult> {
  if (!Number.isFinite(amount) || amount <= 0) {
    return fail("Số tiền rút phải lớn hơn 0.");
  }
  try {
    await requestWalletPayout(sellerId, amount);
  } catch (err: unknown) {
    return fail(messageOf(err, "Yêu cầu rút tiền thất bại."));
  }
  revalidatePath("/seller/wallet");
  return ok();
}

// ── Plans ───────────────────────────────────────────────────────────────────

export async function subscribeAction(planId: string): Promise<ActionResult> {
  const res = await subscribe(planId);
  if (!res.ok) return fail(res.message || "Đăng ký gói thất bại.");
  revalidatePath("/seller/plans");
  return ok();
}

// ── Listing bundles ─────────────────────────────────────────────────────────

export async function createBundleAction(
  title: string,
  listingIds: string[],
  bundlePrice: number,
): Promise<ActionResult<{ bundle: ViewBundle }>> {
  if (!title.trim()) return fail("Nhập tên combo.");
  if (listingIds.length < 2) return fail("Chọn ít nhất 2 sản phẩm cho combo.");
  if (!(bundlePrice > 0)) return fail("Giá combo phải lớn hơn 0.");
  try {
    const bundle = await createBundle(title, listingIds, bundlePrice);
    revalidatePath("/seller/bundles");
    return ok({ bundle });
  } catch (err: unknown) {
    return fail(messageOf(err, "Tạo combo thất bại."));
  }
}

// ── Sponsored ad campaigns ──────────────────────────────────────────────────

export async function createAdCampaignAction(
  listingId: string,
  budget: number,
  bid: number,
): Promise<ActionResult<{ campaign: ViewAdCampaign }>> {
  if (!listingId) return fail("Chọn sản phẩm cần quảng cáo.");
  if (!(budget > 0) || !(bid > 0)) {
    return fail("Ngân sách và giá thầu phải lớn hơn 0.");
  }
  try {
    const campaign = await createAdCampaign(listingId, budget, bid);
    revalidatePath("/seller/ads");
    return ok({ campaign });
  } catch (err: unknown) {
    return fail(messageOf(err, "Tạo chiến dịch thất bại."));
  }
}

// ── Shop profile ────────────────────────────────────────────────────────────

/** Storefront slug is required by team-domain; derived from the seller id. */
function defaultSlug(sellerId: string): string {
  return `shop-${sellerId
    .toLowerCase()
    .replace(/[^a-z0-9]/g, "")
    .slice(0, 12)}`;
}

/**
 * Save the shop display name through UpsertStorefront. The upsert REPLACES the
 * whole storefront, so the current one is read first and only `displayName`
 * changes; the seller id always comes from the session, never from the client.
 */
export async function upsertStorefrontAction(
  displayName: string,
): Promise<ActionResult<{ displayName: string }>> {
  const invalid = validateShopName(displayName);
  if (invalid) return fail(invalid);
  const name = displayName.trim();

  const me = getPrincipal();
  if (!me) return fail("Bạn cần đăng nhập để cập nhật gian hàng.");

  let current: Awaited<ReturnType<typeof getStorefront>>;
  try {
    current = await getStorefront(me.id, { throwOnError: true });
  } catch {
    return fail("Không tải được hồ sơ gian hàng hiện tại. Vui lòng thử lại.");
  }

  try {
    const saved = await upsertStorefront({
      ...(current ?? {}),
      sellerId: me.id,
      slug: current?.slug || defaultSlug(me.id),
      displayName: name,
    });
    revalidatePath("/seller", "layout");
    revalidatePath(`/shop/${me.id}`);
    return ok({ displayName: saved.displayName });
  } catch (err: unknown) {
    return fail(messageOf(err, "Cập nhật tên gian hàng thất bại."));
  }
}
