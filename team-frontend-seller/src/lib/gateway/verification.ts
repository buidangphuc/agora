import "server-only";

import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { VerificationService } from "@/generated/platform/verification/v1/verification_connect.js";
import type { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

export interface ViewVerificationStatus {
  status: VerificationStatus;
  badge: boolean;
  rejectReasonCode: string;
}

export interface ViewKycUpload {
  uploadUrl: string;
  docRef: string;
}

function verification() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(VerificationService, transport);
}

export async function getVerificationStatus(
  userId: string,
): Promise<ViewVerificationStatus> {
  const res = await verification().getVerificationStatus({ userId });
  return {
    status: res.status,
    badge: res.badge,
    rejectReasonCode: res.rejectReasonCode,
  };
}

export async function createKycUploadUrl(
  contentType: string,
  sizeBytes: number,
): Promise<ViewKycUpload> {
  const res = await verification().createKycUploadUrl({
    contentType,
    sizeBytes: BigInt(sizeBytes),
  });
  return { uploadUrl: res.uploadUrl, docRef: res.docRef };
}

export async function submitKyc(
  docType: string,
  docRef: string,
  additionalDocRefs: string[],
) {
  return verification().submitKyc({ docType, docRef, additionalDocRefs });
}
