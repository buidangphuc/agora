import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import {
  DiscountType,
  type Voucher,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export type VoucherScopeName = "platform" | "shop" | "unspecified";
export type DiscountTypeName = "percent" | "fixed" | "unspecified";

export interface VoucherView {
  id: string;
  code: string;
  scope: VoucherScopeName;
  sellerId: string;
  discountType: DiscountTypeName;
  discountValue: number;
  minSpend: number;
  maxDiscount: number;
  quota: number;
  used: number;
  startsAt: string;
  endsAt: string;
}

export type VoucherErrorStatus =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "not_found"
  | "unavailable"
  | "timeout"
  | "failed";

export type ListVouchersResult =
  | { status: "ok"; vouchers: VoucherView[]; nextCursor: string }
  | { status: VoucherErrorStatus };

export type GetVoucherResult =
  | { status: "ok"; voucher: VoucherView }
  | { status: VoucherErrorStatus };

export type CreateVoucherResult =
  | { status: "ok"; voucher: VoucherView }
  | { status: VoucherErrorStatus };

export interface CreatePlatformVoucherInput {
  code: string;
  discountType: "percent" | "fixed";
  discountValue: number;
  minSpend: number;
  maxDiscount: number;
  quota: number;
  startsAt: Date;
  endsAt: Date;
}

export const VOUCHER_PAGE_SIZE = 20;
const READ_TIMEOUT_MS = 5000;
const WRITE_TIMEOUT_MS = 10000;

function scopeName(s: VoucherScope): VoucherScopeName {
  if (s === VoucherScope.PLATFORM) return "platform";
  if (s === VoucherScope.SHOP) return "shop";
  return "unspecified";
}

function typeName(t: DiscountType): DiscountTypeName {
  if (t === DiscountType.PERCENT) return "percent";
  if (t === DiscountType.FIXED) return "fixed";
  return "unspecified";
}

function iso(ts?: Timestamp): string {
  return ts ? ts.toDate().toISOString() : "";
}

export function toVoucherView(v: Voucher): VoucherView {
  return {
    id: v.id,
    code: v.code,
    scope: scopeName(v.scope),
    sellerId: v.sellerId,
    discountType: typeName(v.discountType),
    discountValue: Number(v.discountValue),
    minSpend: Number(v.minSpend),
    maxDiscount: Number(v.maxDiscount),
    quota: Number(v.quota),
    used: Number(v.used),
    startsAt: iso(v.startsAt),
    endsAt: iso(v.endsAt),
  };
}

function errorStatus(err: unknown): VoucherErrorStatus {
  if (!(err instanceof ConnectError)) return "unavailable";
  switch (err.code) {
    case Code.Unauthenticated:
      return "unauthenticated";
    case Code.PermissionDenied:
      return "forbidden";
    case Code.InvalidArgument:
      return "invalid";
    case Code.NotFound:
      return "not_found";
    case Code.Unavailable:
      return "unavailable";
    case Code.DeadlineExceeded:
    case Code.Canceled:
      return "timeout";
    default:
      return "failed";
  }
}

export async function listPlatformVouchers(
  cursor: string,
): Promise<ListVouchersResult> {
  try {
    const { voucher } = makeClients(getToken());
    const res = await voucher.listVouchers(
      { sellerId: "", page: { cursor, pageSize: VOUCHER_PAGE_SIZE } },
      { timeoutMs: READ_TIMEOUT_MS },
    );
    return {
      status: "ok",
      vouchers: res.vouchers.map(toVoucherView),
      nextCursor: res.page?.nextCursor ?? "",
    };
  } catch (err) {
    return { status: errorStatus(err) };
  }
}

export async function getVoucher(code: string): Promise<GetVoucherResult> {
  try {
    const { voucher } = makeClients(getToken());
    const res = await voucher.getVoucher(
      { code },
      { timeoutMs: READ_TIMEOUT_MS },
    );
    if (!res.voucher) return { status: "not_found" };
    return { status: "ok", voucher: toVoucherView(res.voucher) };
  } catch (err) {
    return { status: errorStatus(err) };
  }
}

export async function createPlatformVoucher(
  input: CreatePlatformVoucherInput,
): Promise<CreateVoucherResult> {
  try {
    const { voucher } = makeClients(getToken());
    const res = await voucher.createVoucher(
      {
        code: input.code,
        scope: VoucherScope.PLATFORM,
        discountType:
          input.discountType === "percent"
            ? DiscountType.PERCENT
            : DiscountType.FIXED,
        discountValue: BigInt(input.discountValue),
        minSpend: BigInt(input.minSpend),
        maxDiscount: BigInt(input.maxDiscount),
        quota: BigInt(input.quota),
        startsAt: Timestamp.fromDate(input.startsAt),
        endsAt: Timestamp.fromDate(input.endsAt),
      },
      { timeoutMs: WRITE_TIMEOUT_MS },
    );
    if (!res.voucher) return { status: "failed" };
    return { status: "ok", voucher: toVoucherView(res.voucher) };
  } catch (err) {
    return { status: errorStatus(err) };
  }
}
