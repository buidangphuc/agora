/**
 * Server-only gateway module for team-referral (ReferralService), reached — like
 * every other domain — ONLY through the gateway (ARCHITECTURE Rule 1). A
 * per-request client is built from the caller's httpOnly `session` cookie,
 * mirroring sessions.ts / promotion.ts (ReferralService is not part of the shared
 * makeClients bundle, so it stays a self-contained gateway module). No business
 * logic here: team-referral owns codes/redemptions/rewards; this module forwards
 * and maps proto → plain view types.
 */
import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { ReferralService } from "@/generated/platform/referral/v1/referral_connect.js";
import type { ReferralReward } from "@/generated/platform/referral/v1/referral_pb.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

function referral() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(ReferralService, transport);
}

export interface ViewReferral {
  code: string;
  invitedCount: number;
  rewardsTotal: number; // minor units
}

export interface ViewReferralReward {
  id: string;
  amount: number; // minor units
  reason: string;
  createdAt: string;
}

function tsToString(ts?: Timestamp): string {
  if (!ts) return "";
  return new Date(Number(ts.seconds) * 1000).toLocaleString("vi-VN");
}

function mapReward(r: ReferralReward): ViewReferralReward {
  return {
    id: r.id,
    amount: Number(r.amount),
    reason: r.reason,
    createdAt: tsToString(r.createdAt),
  };
}

/** Mint (or return the existing) referral code for the calling user. */
export async function createReferralCode(): Promise<string> {
  const res = await referral().createReferralCode({});
  return res.code;
}

export async function getMyReferral(): Promise<ViewReferral> {
  try {
    const res = await referral().getMyReferral({});
    return {
      code: res.code,
      invitedCount: Number(res.invitedCount),
      rewardsTotal: Number(res.rewardsTotal),
    };
  } catch {
    return { code: "", invitedCount: 0, rewardsTotal: 0 };
  }
}

export async function redeemReferral(code: string): Promise<void> {
  await referral().redeemReferral({ code: code.trim() });
}

export async function listReferralRewards(): Promise<ViewReferralReward[]> {
  try {
    const res = await referral().listReferralRewards({
      page: { pageSize: 50 },
    });
    return res.rewards.map(mapReward);
  } catch {
    return [];
  }
}
