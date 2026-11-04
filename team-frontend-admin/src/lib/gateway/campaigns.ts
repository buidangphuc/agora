import "server-only";

import { ScalarType, Timestamp, proto3 } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import {
  CampaignStatus,
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
  status: CampaignStatusName;
  cancelledAt: string;
}

export type CampaignStatusName =
  "scheduled" | "active" | "ended" | "cancelled" | "unspecified";

export type CampaignFilterStatus = Exclude<CampaignStatusName, "unspecified">;

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

export interface UpdateCampaignInput {
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
  { status: "ok"; plans: PlanView[] } | { status: ReadFailure };

export type CampaignWriteFailure =
  | "unauthenticated"
  | "forbidden"
  | "invalid"
  | "not_found"
  | "duplicate"
  | "listing_unavailable"
  | "audit_unavailable"
  | "promotion_unavailable"
  | "unavailable"
  | "timeout";

export type CreateCampaignResult =
  { status: "ok"; campaign: CampaignView } | { status: CampaignWriteFailure };

export type CancelCampaignResult = CreateCampaignResult;
export type UpdateCampaignResult = CreateCampaignResult;

export type ListCampaignsResult =
  | {
      status: "ok";
      campaigns: CampaignView[];
      nextCursor: string;
      flashSaleDisabled: boolean;
    }
  | { status: ReadFailure };

export const CAMPAIGN_PAGE_SIZE = 20;
const READ_TIMEOUT_MS = 5000;
const WRITE_TIMEOUT_MS = 10000;

const TIER: Record<PlanTier, PlanTierName> = {
  [PlanTier.UNSPECIFIED]: "unspecified",
  [PlanTier.FREE]: "free",
  [PlanTier.PRO]: "pro",
  [PlanTier.PREMIUM]: "premium",
};

const STATUS_NAME: Record<CampaignStatus, CampaignStatusName> = {
  [CampaignStatus.UNSPECIFIED]: "unspecified",
  [CampaignStatus.SCHEDULED]: "scheduled",
  [CampaignStatus.ACTIVE]: "active",
  [CampaignStatus.ENDED]: "ended",
  [CampaignStatus.CANCELLED]: "cancelled",
};

const STATUS_ENUM: Record<CampaignFilterStatus, CampaignStatus> = {
  scheduled: CampaignStatus.SCHEDULED,
  active: CampaignStatus.ACTIVE,
  ended: CampaignStatus.ENDED,
  cancelled: CampaignStatus.CANCELLED,
};

const ErrorInfo = proto3.makeMessageType("google.rpc.ErrorInfo", [
  { no: 1, name: "reason", kind: "scalar", T: ScalarType.STRING },
  { no: 2, name: "domain", kind: "scalar", T: ScalarType.STRING },
]);

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
    status: STATUS_NAME[c.status] ?? "unspecified",
    cancelledAt: iso(c.cancelledAt),
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

function unavailableStatus(err: ConnectError): CampaignWriteFailure {
  const reason = err.findDetails(ErrorInfo)[0]?.reason;
  if (reason === "LISTING_UNAVAILABLE") return "listing_unavailable";
  if (reason === "AUDIT_UNAVAILABLE") return "audit_unavailable";
  if (reason === "PROMOTION_UNAVAILABLE") return "promotion_unavailable";
  return "unavailable";
}

function writeFailure(err: unknown): CampaignWriteFailure {
  if (!(err instanceof ConnectError)) return "unavailable";
  switch (err.code) {
    case Code.Unauthenticated:
      return "unauthenticated";
    case Code.PermissionDenied:
      return "forbidden";
    case Code.InvalidArgument:
    case Code.FailedPrecondition:
      return "invalid";
    case Code.NotFound:
      return "not_found";
    case Code.AlreadyExists:
      return "duplicate";
    case Code.DeadlineExceeded:
      return "timeout";
    case Code.Unavailable:
      return unavailableStatus(err);
    default:
      return "unavailable";
  }
}

export async function listCampaigns(
  statuses: CampaignFilterStatus[],
  cursor: string,
): Promise<ListCampaignsResult> {
  try {
    const { flashSale } = makeClients(getToken());
    const res = await flashSale.listCampaigns(
      {
        statuses: statuses.map((s) => STATUS_ENUM[s]),
        page: { cursor, pageSize: CAMPAIGN_PAGE_SIZE },
      },
      { timeoutMs: READ_TIMEOUT_MS },
    );
    return {
      status: "ok",
      campaigns: res.campaigns.map(toCampaign),
      nextCursor: res.page?.nextCursor ?? "",
      flashSaleDisabled: res.flashSaleDisabled,
    };
  } catch (err) {
    return { status: readFailure(err) };
  }
}

export async function createCampaign(
  input: CreateCampaignInput,
  idempotencyKey = "",
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
        idempotencyKey,
      },
      { timeoutMs: WRITE_TIMEOUT_MS },
    );
    if (!res.campaign) return { status: "unavailable" };
    return { status: "ok", campaign: toCampaign(res.campaign) };
  } catch (err) {
    return { status: writeFailure(err) };
  }
}

export async function cancelCampaign(
  campaignId: string,
): Promise<CancelCampaignResult> {
  try {
    const { flashSale } = makeClients(getToken());
    const res = await flashSale.cancelCampaign(
      { campaignId },
      { timeoutMs: WRITE_TIMEOUT_MS },
    );
    if (!res.campaign) return { status: "unavailable" };
    return { status: "ok", campaign: toCampaign(res.campaign) };
  } catch (err) {
    return { status: writeFailure(err) };
  }
}

export async function updateCampaign(
  campaignId: string,
  input: UpdateCampaignInput,
): Promise<UpdateCampaignResult> {
  try {
    const { flashSale } = makeClients(getToken());
    const res = await flashSale.updateCampaign(
      {
        campaignId,
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
    return { status: writeFailure(err) };
  }
}
