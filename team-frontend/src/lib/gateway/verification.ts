/**
 * Server-only gateway module for team-verification (VerificationService), reached
 * — like every other domain — ONLY through the gateway (ARCHITECTURE Rule 1). A
 * per-request client is built from the caller's httpOnly `session` cookie,
 * mirroring sessions.ts / promotion.ts (VerificationService is not part of the
 * shared makeClients bundle, so it stays a self-contained gateway module). No
 * business logic here: team-verification owns KYC review; this module forwards
 * and maps proto → plain view types.
 */
import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { VerificationService } from "@/generated/platform/verification/v1/verification_connect.js";
import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getPrincipal, getToken } from "./session.js";

function verification() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(VerificationService, transport);
}

export { VerificationStatus };

export interface ViewVerificationStatus {
  status: VerificationStatus;
  statusText: string;
  badge: boolean;
}

export function verificationStatusText(s: VerificationStatus): string {
  switch (s) {
    case VerificationStatus.PENDING:
      return "Đang chờ duyệt";
    case VerificationStatus.VERIFIED:
      return "Đã xác minh";
    case VerificationStatus.REJECTED:
      return "Bị từ chối";
    default:
      return "Chưa xác minh";
  }
}

export interface SubmitKycResult {
  id: string;
  status: VerificationStatus;
}

/** Submit a KYC document for review (called by the user being verified). */
export async function submitKyc(
  docType: string,
  docRef: string,
): Promise<SubmitKycResult> {
  const res = await verification().submitKyc({
    docType: docType.trim(),
    docRef: docRef.trim(),
  });
  return { id: res.id, status: res.status };
}

/** Current verification status for the calling user (or a given user id). */
export async function getVerificationStatus(
  userId?: string,
): Promise<ViewVerificationStatus> {
  const uid = userId ?? getPrincipal()?.id ?? "";
  try {
    const res = await verification().getVerificationStatus({ userId: uid });
    return {
      status: res.status,
      statusText: verificationStatusText(res.status),
      badge: res.badge,
    };
  } catch {
    return {
      status: VerificationStatus.UNSPECIFIED,
      statusText: verificationStatusText(VerificationStatus.UNSPECIFIED),
      badge: false,
    };
  }
}
