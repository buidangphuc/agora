"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type ListingPage,
  type ViewBundle,
  createBundle,
  getStorefront,
  listMyListings,
  upsertStorefront,
} from "@/lib/gateway/listings";
import { type ViewOrderReturn, updateReturnStatus } from "@/lib/gateway/orders";
import { requestWalletPayout } from "@/lib/gateway/payment";

import {
  DiscountType,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";
import {
  type CreateCampaignInput,
  type ViewAdCampaign,
  cancelMyCampaign,
  cancelSubscription,
  createAdCampaign,
  createCampaign,
  createVoucher,
  deactivateMyShopVoucher,
  pauseAdCampaign,
  resumeAdCampaign,
  subscribe,
} from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import { FEATURED_MAX, validateTagline, validateTheme } from "./shopBranding";
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

// ── Returns (RMA) ───────────────────────────────────────────────────────────

/**
 * Move one return through `UpdateReturnStatus` and nothing else: for REFUNDED
 * the order service emits the fact and the payment applies it. The page is
 * revalidated on failure too, so it shows the status the gateway reports.
 */
async function moveReturn(
  returnId: string,
  orderId: string,
  status: ReturnStatus,
  failure: string,
): Promise<ActionResult<ViewOrderReturn>> {
  try {
    const ret = await updateReturnStatus(returnId, status);
    return ok(ret);
  } catch (err: unknown) {
    return fail(messageOf(err, failure));
  } finally {
    revalidatePath(`/seller/orders/${orderId}`);
    revalidatePath("/seller/orders");
  }
}

export async function approveReturnAction(
  returnId: string,
  orderId: string,
): Promise<ActionResult<ViewOrderReturn>> {
  return moveReturn(
    returnId,
    orderId,
    ReturnStatus.APPROVED,
    "Duyệt thất bại.",
  );
}

export async function rejectReturnAction(
  returnId: string,
  orderId: string,
): Promise<ActionResult<ViewOrderReturn>> {
  return moveReturn(
    returnId,
    orderId,
    ReturnStatus.REJECTED,
    "Từ chối thất bại.",
  );
}

export async function refundReturnAction(
  returnId: string,
  orderId: string,
): Promise<ActionResult<ViewOrderReturn>> {
  return moveReturn(
    returnId,
    orderId,
    ReturnStatus.REFUNDED,
    "Hoàn tiền thất bại.",
  );
}

// ── Plans ───────────────────────────────────────────────────────────────────

export async function subscribeAction(planId: string): Promise<ActionResult> {
  try {
    const res = await subscribe(planId);
    if (!res.ok) return fail(res.message || "Đăng ký gói thất bại.");
  } catch (err: unknown) {
    if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
      return fail("Số dư có thể rút không đủ để trả gói này.");
    }
    return fail("Đăng ký gói thất bại.");
  }
  revalidatePath("/seller/plans");
  return ok();
}

export async function cancelSubscriptionAction(): Promise<ActionResult> {
  try {
    await cancelSubscription();
  } catch {
    return fail("Hủy gia hạn thất bại.");
  }
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
  if (bid < 500) return fail("Giá thầu tối thiểu là ₫500.");
  try {
    const campaign = await createAdCampaign(listingId, budget, bid);
    revalidatePath("/seller/ads");
    return ok({ campaign });
  } catch (err: unknown) {
    return fail(messageOf(err, "Tạo chiến dịch thất bại."));
  }
}

async function moveCampaign(
  campaignId: string,
  op: (id: string) => Promise<ViewAdCampaign>,
  failure: string,
): Promise<ActionResult<{ campaign: ViewAdCampaign }>> {
  try {
    const campaign = await op(campaignId);
    revalidatePath("/seller/ads");
    return ok({ campaign });
  } catch (err: unknown) {
    if (err instanceof ConnectError) {
      if (err.code === Code.NotFound) return fail("Không tìm thấy chiến dịch.");
      if (err.code === Code.FailedPrecondition) {
        return fail(
          "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy.",
        );
      }
    }
    return fail(failure);
  }
}

export async function pauseAdCampaignAction(
  campaignId: string,
): Promise<ActionResult<{ campaign: ViewAdCampaign }>> {
  return moveCampaign(campaignId, pauseAdCampaign, "Tạm dừng thất bại.");
}

export async function resumeAdCampaignAction(
  campaignId: string,
): Promise<ActionResult<{ campaign: ViewAdCampaign }>> {
  return moveCampaign(campaignId, resumeAdCampaign, "Tiếp tục thất bại.");
}

// ── Shop profile ────────────────────────────────────────────────────────────

/** Storefront slug is required by team-domain; derived from the seller id. */
function defaultSlug(sellerId: string): string {
  return `shop-${sellerId
    .toLowerCase()
    .replace(/[^a-z0-9]/g, "")
    .slice(0, 12)}`;
}

export interface StorefrontInput {
  displayName?: string;
  bannerUrl?: string;
  tagline?: string;
  featuredListingIds?: string[];
  theme?: string;
}

/**
 * Save the storefront through UpsertStorefront. The upsert REPLACES the whole
 * storefront, so the current one is read first and only the given fields
 * change; the seller id always comes from the session, never from the client.
 */
export async function upsertStorefrontAction(
  raw: string | StorefrontInput,
): Promise<ActionResult<{ displayName: string }>> {
  const input: StorefrontInput =
    typeof raw === "string" ? { displayName: raw } : raw;

  let displayName: string | undefined;
  if (input.displayName !== undefined) {
    const invalid = validateShopName(input.displayName);
    if (invalid) return fail(invalid);
    displayName = input.displayName.trim();
  }
  let tagline: string | undefined;
  if (input.tagline !== undefined) {
    const invalid = validateTagline(input.tagline);
    if (invalid) return fail(invalid);
    tagline = input.tagline.trim();
  }
  if (input.theme !== undefined) {
    const invalid = validateTheme(input.theme);
    if (invalid) return fail(invalid);
  }
  if (
    input.featuredListingIds !== undefined &&
    input.featuredListingIds.length > FEATURED_MAX
  ) {
    return fail("Tối đa 6 sản phẩm nổi bật.");
  }

  const me = getPrincipal();
  if (!me) return fail("Bạn cần đăng nhập để cập nhật gian hàng.");
  if (!me.scopes.includes("listing.write")) {
    return fail("Bạn không có quyền cập nhật gian hàng.");
  }

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
      ...(displayName !== undefined && { displayName }),
      ...(tagline !== undefined && { tagline }),
      ...(input.bannerUrl !== undefined && { bannerUrl: input.bannerUrl }),
      ...(input.featuredListingIds !== undefined && {
        featuredListingIds: input.featuredListingIds,
      }),
      ...(input.theme !== undefined && { theme: input.theme }),
    });
    revalidatePath("/seller");
    revalidatePath(`/shop/${me.id}`);
    return ok({ displayName: saved.displayName });
  } catch (err: unknown) {
    return fail(messageOf(err, "Cập nhật gian hàng thất bại."));
  }
}

const SEARCH_QUERY_MAX = 100;
const SEARCH_PAGE_SIZE = 20;

export async function searchMyListingsAction(
  query: string,
): Promise<ActionResult<{ page: ListingPage }>> {
  const q = query.trim();
  if ([...q].length > SEARCH_QUERY_MAX) {
    return fail("Từ khóa tìm kiếm tối đa 100 ký tự.");
  }
  const me = getPrincipal();
  if (!me?.scopes.includes("listing.write")) {
    return fail("Bạn không có quyền xem danh sách sản phẩm.");
  }
  try {
    const page = await listMyListings({
      statuses: [ListingStatus.PUBLISHED],
      query: q,
      pageSize: SEARCH_PAGE_SIZE,
    });
    return ok({ page });
  } catch {
    return fail("Không tải được danh sách sản phẩm.");
  }
}

// ── Marketing hub (shop vouchers + flash-sale campaigns) ────────────────────

const MARKETING_PATH = "/seller/marketing";

function canWrite(): boolean {
  return Boolean(getPrincipal()?.scopes.includes("listing.write"));
}

const NO_WRITE = "Bạn không có quyền thực hiện thao tác này.";

export interface ShopVoucherInput {
  code: string;
  discountType: DiscountType;
  discountValue: number;
  minSpend: number;
  maxDiscount: number;
  quota: number;
  startsAt: Date;
  endsAt: Date;
}

export async function createShopVoucherAction(
  input: ShopVoucherInput,
): Promise<ActionResult> {
  if (!canWrite()) return fail(NO_WRITE);
  try {
    await createVoucher({ ...input, scope: VoucherScope.SHOP });
  } catch (err: unknown) {
    if (err instanceof ConnectError && err.code === Code.AlreadyExists) {
      return fail("Mã voucher đã tồn tại");
    }
    const msg =
      err instanceof ConnectError ? err.rawMessage : messageOf(err, "");
    return fail(msg || "Tạo voucher thất bại.");
  }
  revalidatePath(MARKETING_PATH);
  return ok();
}

export async function deactivateShopVoucherAction(
  voucherId: string,
): Promise<ActionResult> {
  if (!canWrite()) return fail(NO_WRITE);
  const res = await deactivateMyShopVoucher(voucherId);
  if (!res.ok) {
    if (res.code === "denied" || res.code === "not_found") {
      return fail("Không có quyền hoặc voucher không tồn tại");
    }
    return fail(res.message || "Tắt voucher thất bại.");
  }
  revalidatePath(MARKETING_PATH);
  return ok();
}

export async function createCampaignAction(
  input: CreateCampaignInput,
): Promise<ActionResult> {
  if (!canWrite()) return fail(NO_WRITE);
  const res = await createCampaign(input);
  if (!res.ok) {
    if (res.code === "denied") {
      return fail("Không có quyền tạo flash sale cho sản phẩm này");
    }
    return fail(res.message || "Tạo flash sale thất bại.");
  }
  revalidatePath(MARKETING_PATH);
  return ok();
}

export async function cancelCampaignAction(
  campaignId: string,
): Promise<ActionResult> {
  if (!canWrite()) return fail(NO_WRITE);
  const res = await cancelMyCampaign(campaignId);
  if (!res.ok) {
    if (res.code === "denied" || res.code === "not_found") {
      return fail("Không có quyền hoặc flash sale không tồn tại");
    }
    if (res.code === "failed_precondition") {
      revalidatePath(MARKETING_PATH);
      return fail("Flash sale đã bắt đầu, không thể huỷ");
    }
    return fail(res.message || "Huỷ flash sale thất bại.");
  }
  revalidatePath(MARKETING_PATH);
  return ok();
}
