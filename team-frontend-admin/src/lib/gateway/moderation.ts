/**
 * Admin listing-moderation calls over the Gateway's ListingService.
 * server-only; never throws and never returns an error message (it could carry
 * upstream addresses). The gateway enforces the `admin` scope.
 */
import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import { ModerationDecision } from "@/generated/platform/listing/v1/listing_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export interface ModerationQueueItemView {
  id: string;
  title: string;
  sellerId: string;
  price: number;
  /** ISO-8601 UTC. */
  submittedAt: string;
  screenFlags: string[];
}

export type ModerationQueueResult =
  | { status: "ok"; items: ModerationQueueItemView[]; nextCursor: string }
  | { status: "unauthenticated" | "forbidden" | "unavailable" };

export type ModerationDecisionName = "approve" | "reject" | "ban";

export type ModerateResult =
  | { status: "ok" }
  | {
      status:
        | "unauthenticated"
        | "forbidden"
        | "not_found"
        | "failed_precondition"
        | "invalid"
        | "unavailable";
    };

const PAGE_SIZE = 20;
const TIMEOUT_MS = 5000;

const DECISIONS: Record<ModerationDecisionName, ModerationDecision> = {
  approve: ModerationDecision.APPROVE,
  reject: ModerationDecision.REJECT,
  ban: ModerationDecision.BAN,
};

export async function listModerationQueue(
  cursor: string,
): Promise<ModerationQueueResult> {
  try {
    const { listing } = makeClients(getToken());
    const res = await listing.listModerationQueue(
      { page: { cursor, pageSize: PAGE_SIZE } },
      { timeoutMs: TIMEOUT_MS },
    );
    return {
      status: "ok",
      nextCursor: res.page?.nextCursor ?? "",
      items: res.items.map((i) => ({
        id: i.listing?.id ?? "",
        title: i.listing?.title ?? "",
        sellerId: i.listing?.sellerId ?? "",
        price: Number(i.listing?.price ?? 0n),
        submittedAt: i.submittedAt?.toDate().toISOString() ?? "",
        screenFlags: [...(i.listing?.moderation?.screenFlags ?? [])],
      })),
    };
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.Unauthenticated)
        return { status: "unauthenticated" };
      if (err.code === Code.PermissionDenied) return { status: "forbidden" };
    }
    return { status: "unavailable" };
  }
}

export async function moderateListing(
  listingId: string,
  decision: ModerationDecisionName,
  reasonCode: string,
  note: string,
): Promise<ModerateResult> {
  try {
    const { listing } = makeClients(getToken());
    await listing.moderateListing(
      { listingId, decision: DECISIONS[decision], reasonCode, note },
      { timeoutMs: TIMEOUT_MS },
    );
    return { status: "ok" };
  } catch (err) {
    if (err instanceof ConnectError) {
      switch (err.code) {
        case Code.Unauthenticated:
          return { status: "unauthenticated" };
        case Code.PermissionDenied:
          return { status: "forbidden" };
        case Code.NotFound:
          return { status: "not_found" };
        case Code.FailedPrecondition:
          return { status: "failed_precondition" };
        case Code.InvalidArgument:
          return { status: "invalid" };
      }
    }
    return { status: "unavailable" };
  }
}
