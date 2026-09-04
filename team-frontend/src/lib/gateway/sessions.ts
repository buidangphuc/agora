/**
 * Server-only gateway module for team-identity SessionService, reached — like
 * every other domain — ONLY through the gateway (ARCHITECTURE Rule 1). A
 * per-request client is built from the caller's httpOnly `session` cookie,
 * mirroring promotion.ts (SessionService is not part of the shared makeClients
 * bundle, so it stays a self-contained gateway module). No business logic here:
 * team-identity owns sessions/login history; this module forwards and maps
 * proto → plain view types.
 */
import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";

import { SessionService } from "@/generated/platform/identity/v1/identity_connect.js";
import type {
  LoginEvent,
  Session,
} from "@/generated/platform/identity/v1/identity_pb.js";

import { authInterceptor } from "./auth.js";
import { gatewayConfig } from "./config.js";
import { getToken } from "./session.js";

function sessions() {
  const transport = createConnectTransport({
    baseUrl: gatewayConfig.gatewayUrl,
    httpVersion: "1.1",
    interceptors: [authInterceptor({ token: getToken() })],
  });
  return createPromiseClient(SessionService, transport);
}

export interface ViewSession {
  id: string;
  device: string;
  ip: string;
  createdAt: string;
  lastSeen: string;
  revoked: boolean;
}

export interface ViewLoginEvent {
  id: string;
  ip: string;
  userAgent: string;
  success: boolean;
  createdAt: string;
}

function tsToString(ts?: Timestamp): string {
  if (!ts) return "";
  return new Date(Number(ts.seconds) * 1000).toLocaleString("vi-VN");
}

function mapSession(s: Session): ViewSession {
  return {
    id: s.id,
    device: s.device,
    ip: s.ip,
    createdAt: tsToString(s.createdAt),
    lastSeen: tsToString(s.lastSeen),
    revoked: s.revoked,
  };
}

function mapLoginEvent(e: LoginEvent): ViewLoginEvent {
  return {
    id: e.id,
    ip: e.ip,
    userAgent: e.userAgent,
    success: e.success,
    createdAt: tsToString(e.createdAt),
  };
}

export async function listSessions(): Promise<ViewSession[]> {
  try {
    const res = await sessions().listSessions({});
    return res.sessions.map(mapSession);
  } catch {
    return [];
  }
}

export async function revokeSession(sessionId: string): Promise<void> {
  await sessions().revokeSession({ sessionId });
}

export async function listLoginHistory(): Promise<ViewLoginEvent[]> {
  try {
    const res = await sessions().listLoginHistory({});
    return res.events.map(mapLoginEvent);
  } catch {
    return [];
  }
}
