/**
 * Server-only gateway module for team-promotion (VoucherService + FlashSaleService),
 * reached — like every other domain — ONLY through the gateway (ARCHITECTURE Rule 1).
 * A per-request client is built from the caller's httpOnly `session` cookie, mirroring
 * orders.ts / cart.ts. No business logic lives here: discounts are computed by
 * team-promotion; this module just forwards and maps proto → plain view types.
 */
import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import {
  FlashSaleService,
  SponsoredService,
  VoucherService,
} from "@/generated/platform/promotion/v1/promotion_connect.js";
import {
  DiscountType,
  type FlashSaleCampaign,
  type Voucher,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getPrincipal, getToken } from "./session.js";

// Build the promotion clients per request with the caller's bearer, using the
// same single gateway hop as client.ts (kept local so promotion stays a
// self-contained gateway module).
function promotion() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return {
    voucher: createPromiseClient(VoucherService, transport),
    flashSale: createPromiseClient(FlashSaleService, transport),
    sponsored: createPromiseClient(SponsoredService, transport),
  };
}

export { DiscountType, VoucherScope };

export interface ViewVoucher {
  id: string;
  code: string;
  scope: VoucherScope;
  scopeText: string;
  sellerId: string;
  discountType: DiscountType;
  discountTypeText: string;
  discountValue: number;
  minSpend: number;
  maxDiscount: number;
  quota: number;
  used: number;
  startsAt: string;
  endsAt: string;
}

export interface VoucherPreview {
  valid: boolean;
  reason: string;
  discountAmount: number;
  voucherId: string;
}

export interface ViewFlashSale {
  active: boolean;
  campaign?: ViewFlashSaleCampaign;
}

export interface ViewFlashSaleCampaign {
  id: string;
  listingId: string;
  variantId: string;
  salePrice: number;
  stockCap: number;
  stockSold: number;
  remaining: number;
}

export interface ViewFlashSaleStock {
  remaining: number;
  stockCap: number;
}

function discountTypeText(t: DiscountType): string {
  switch (t) {
    case DiscountType.PERCENT:
      return "Giảm %";
    case DiscountType.FIXED:
      return "Giảm tiền";
    default:
      return "Không xác định";
  }
}

function scopeText(s: VoucherScope): string {
  switch (s) {
    case VoucherScope.SHOP:
      return "Shop";
    case VoucherScope.PLATFORM:
      return "Toàn sàn";
    default:
      return "Không xác định";
  }
}

function tsToString(ts?: Timestamp): string {
  if (!ts) return "";
  return new Date(Number(ts.seconds) * 1000).toLocaleDateString("vi-VN");
}

function mapVoucher(v: Voucher): ViewVoucher {
  return {
    id: v.id,
    code: v.code,
    scope: v.scope,
    scopeText: scopeText(v.scope),
    sellerId: v.sellerId,
    discountType: v.discountType,
    discountTypeText: discountTypeText(v.discountType),
    discountValue: Number(v.discountValue),
    minSpend: Number(v.minSpend),
    maxDiscount: Number(v.maxDiscount),
    quota: Number(v.quota),
    used: Number(v.used),
    startsAt: tsToString(v.startsAt),
    endsAt: tsToString(v.endsAt),
  };
}

function mapCampaign(c: FlashSaleCampaign): ViewFlashSaleCampaign {
  const cap = Number(c.stockCap);
  const sold = Number(c.stockSold);
  return {
    id: c.id,
    listingId: c.listingId,
    variantId: c.variantId,
    salePrice: Number(c.salePrice),
    stockCap: cap,
    stockSold: sold,
    remaining: Math.max(0, cap - sold),
  };
}

/**
 * Preview the discount for a voucher code before the order is placed. Mirrors the
 * checkout seam: ValidateAndReserve is idempotent on reservation_id, so a stable
 * per-(buyer, code) preview id lets the buyer re-apply without reserving twice.
 * The order saga performs its own authoritative reserve→commit/release later.
 */
export async function previewVoucher(
  code: string,
  cartSubtotal: number,
  sellerId?: string,
): Promise<VoucherPreview> {
  const me = getPrincipal();
  const buyerId = me?.id ?? "";
  const reservationId = `preview:${buyerId}:${code.trim().toUpperCase()}`;
  try {
    const res = await promotion().voucher.validateAndReserve({
      reservationId,
      code: code.trim(),
      buyerId,
      cartSubtotal: BigInt(Math.max(0, Math.round(cartSubtotal))),
      sellerId: sellerId ?? "",
    });
    return {
      valid: res.valid,
      reason: res.reason,
      discountAmount: Number(res.discountAmount),
      voucherId: res.voucherId,
    };
  } catch (err) {
    return {
      valid: false,
      reason:
        err instanceof Error ? err.message : "Không kiểm tra được mã giảm giá.",
      discountAmount: 0,
      voucherId: "",
    };
  }
}

export async function listVouchers(sellerId?: string): Promise<ViewVoucher[]> {
  try {
    const res = await promotion().voucher.listVouchers({
      sellerId: sellerId ?? "",
    });
    return res.vouchers.map(mapVoucher);
  } catch {
    return [];
  }
}

export interface CreateVoucherInput {
  code: string;
  scope: VoucherScope;
  discountType: DiscountType;
  discountValue: number;
  minSpend: number;
  maxDiscount: number;
  quota: number;
  startsAt?: Date;
  endsAt?: Date;
}

export async function createVoucher(
  input: CreateVoucherInput,
): Promise<ViewVoucher> {
  const startsAt = input.startsAt ?? new Date();
  const endsAt =
    input.endsAt ?? new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);
  const res = await promotion().voucher.createVoucher({
    code: input.code.trim(),
    scope: input.scope,
    discountType: input.discountType,
    discountValue: BigInt(Math.max(0, Math.round(input.discountValue))),
    minSpend: BigInt(Math.max(0, Math.round(input.minSpend))),
    maxDiscount: BigInt(Math.max(0, Math.round(input.maxDiscount))),
    quota: BigInt(Math.max(0, Math.round(input.quota))),
    startsAt: Timestamp.fromDate(startsAt),
    endsAt: Timestamp.fromDate(endsAt),
  });
  if (!res.voucher) throw new Error("create voucher failed");
  return mapVoucher(res.voucher);
}

export async function getActiveFlashSale(
  listingId: string,
): Promise<ViewFlashSale> {
  try {
    const res = await promotion().flashSale.getActiveFlashSale({ listingId });
    return {
      active: res.active,
      campaign: res.campaign ? mapCampaign(res.campaign) : undefined,
    };
  } catch {
    return { active: false };
  }
}

export async function getFlashSaleStock(
  campaignId: string,
): Promise<ViewFlashSaleStock> {
  try {
    const res = await promotion().flashSale.getFlashSaleStock({ campaignId });
    return {
      remaining: Number(res.remaining),
      stockCap: Number(res.stockCap),
    };
  } catch {
    return { remaining: 0, stockCap: 0 };
  }
}

// ── Sponsored placements ────────────────────────────────────────────────────

/** Listing ids to render in sponsored slots for a given context string. */
export async function listSponsoredSlots(
  contextStr: string,
): Promise<string[]> {
  try {
    const res = await promotion().sponsored.listSponsoredSlots({ contextStr });
    return res.listingIds;
  } catch {
    return [];
  }
}
