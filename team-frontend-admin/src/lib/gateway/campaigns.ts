import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import {
  type FlashSaleCampaign,
  type Plan,
  PlanTier,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export interface CampaignView {
  id: string;
  listingId: string;
  variantId: string;
  salePrice: number;
  stockCap: number;
  stockSold: number;
  startsAt: string;
  endsAt: string;
}

export type PlanTierName = "free" | "pro" | "premium" | "unspecified";

export interface PlanView {
  id: string;
  tier: PlanTierName;
  price: number;
  features: string[];
}

export interface CreateCampaignInput {
  listingId: string;
  variantId: string;
  salePrice: number;
  stockCap: number;
  startsAt: Date;
  endsAt: Date;
}

type ReadFailure = "unauthenticated" | "forbidden" | "unavailable";

export type ListActiveCampaignsResult =
  | { status: "ok"; campaigns: CampaignView[]; nextCursor: string }
  | { status: ReadFailure };

export type GetActiveFlashSaleResult =
  | { status: "ok"; active: false }
  | { status: "ok"; active: true; campaign: CampaignView }
  | { status: ReadFailure | "invalid" };

export type ListPlansResult =
  | { status: "ok"; plans: PlanView[] }
  | { status: ReadFailure };

export type CreateCampaignResult =
  | { status: "ok"; campaign: CampaignView }
  | {
      status:
        | "unauthenticated"
        | "forbidden"
        | "invalid"
        | "not_found"
        | "unavailable"
        | "timeout";
    };

export const CAMPAIGN_PAGE_SIZE = 20;
const READ_TIMEOUT_MS = 5000;
const WRITE_TIMEOUT_MS = 10000;

const TIER: Record<PlanTier, PlanTierName> = {
  [PlanTier.UNSPECIFIED]: "unspecified",
  [PlanTier.FREE]: "free",
  [PlanTier.PRO]: "pro",
  [PlanTier.PREMIUM]: "premium",
};

function iso(ts?: Timestamp): string {
  return ts ? ts.toDate().toISOString() : "";
}

function toCampaign(c: FlashSaleCampaign): CampaignView {
  return {
    id: c.id,
    listingId: c.listingId,
    variantId: c.variantId,
    salePrice: Number(c.salePrice),
    stockCap: Number(c.stockCap),
    stockSold: Number(c.stockSold),
    startsAt: iso(c.startsAt),
    endsAt: iso(c.endsAt),
  };
}

function toPlan(p: Plan): PlanView {
  return {
    id: p.id,
    tier: TIER[p.name] ?? "unspecified",
    price: Number(p.price),
    features: [...p.features],
  };
}

function codeOf(err: unknown): Code | undefined {
  return err instanceof ConnectError ? err.code : undefined;
}

function readFailure(err: unknown): ReadFailure {
  const code = codeOf(err);
  if (code === Code.Unauthenticated) return "unauthenticated";
  if (code === Code.PermissionDenied) return "forbidden";
  return "unavailable";
}

export async function listActiveCampaigns(
  cursor: string,
): Promise<ListActiveCampaignsResult> {
  try {
    const { flashSale } = makeClients(getToken());
    const res = await flashSale.listActiveCampaigns(
      { page: { cursor, pageSize: CAMPAIGN_PAGE_SIZE } },
      { timeoutMs: READ_TIMEOUT_MS },
    );
    return {
      status: "ok",
      campaigns: res.campaigns.map(toCampaign),
      nextCursor: res.page?.nextCursor ?? "",
    };
  } catch (err) {
    return { status: readFailure(err) };
  }
}

export async function getActiveFlashSale(
  listingId: string,
): Promise<GetActiveFlashSaleResult> {
  try {
    const { flashSale } = makeClients(getToken());
    const res = await flashSale.getActiveFlashSale(
      { listingId },
      { timeoutMs: READ_TIMEOUT_MS },
    );
    if (res.active && res.campaign) {
      return { status: "ok", active: true, campaign: toCampaign(res.campaign) };
    }
    return { status: "ok", active: false };
  } catch (err) {
    if (codeOf(err) === Code.InvalidArgument) return { status: "invalid" };
    return { status: readFailure(err) };
  }
}

export async function listPlans(): Promise<ListPlansResult> {
  try {
    const { subscription } = makeClients(getToken());
    const res = await subscription.listPlans(
      {},
      { timeoutMs: READ_TIMEOUT_MS },
    );
    return { status: "ok", plans: res.plans.map(toPlan) };
  } catch (err) {
    return { status: readFailure(err) };
  }
}

export async function createCampaign(
  input: CreateCampaignInput,
): Promise<CreateCampaignResult> {
  try {
    const { flashSale } = makeClients(getToken());
    const res = await flashSale.createCampaign(
      {
        listingId: input.listingId,
        variantId: input.variantId,
        salePrice: BigInt(input.salePrice),
        stockCap: BigInt(input.stockCap),
        startsAt: Timestamp.fromDate(input.startsAt),
        endsAt: Timestamp.fromDate(input.endsAt),
      },
      { timeoutMs: WRITE_TIMEOUT_MS },
    );
    if (!res.campaign) return { status: "unavailable" };
    return { status: "ok", campaign: toCampaign(res.campaign) };
  } catch (err) {
    switch (codeOf(err)) {
      case Code.Unauthenticated:
        return { status: "unauthenticated" };
      case Code.PermissionDenied:
        return { status: "forbidden" };
      case Code.InvalidArgument:
        return { status: "invalid" };
      case Code.NotFound:
        return { status: "not_found" };
      case Code.DeadlineExceeded:
        return { status: "timeout" };
      default:
        return { status: "unavailable" };
    }
  }
}
