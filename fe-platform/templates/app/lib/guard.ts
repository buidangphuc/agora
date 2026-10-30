import "server-only";

import {
  type ApiErrorKind,
  createGatewayClients,
  toApiError,
} from "@fe-platform/api-client";
import { notFound, redirect } from "next/navigation";
import { requireSession, safeNext } from "./session";

export type Clients = ReturnType<typeof createGatewayClients>;

function refreshUrl(next: string) {
  return `/session/refresh?next=${encodeURIComponent(safeNext(next))}`;
}

export async function guarded<T>(
  next: string,
  fn: (clients: Clients) => Promise<T>,
): Promise<T> {
  const token = await requireSession(next);
  let failure: ReturnType<typeof toApiError>;
  try {
    return await fn(createGatewayClients({ token }));
  } catch (err) {
    failure = toApiError(err);
  }
  if (failure.kind === "unauthenticated") redirect(refreshUrl(next));
  if (failure.kind === "not_found") notFound();
  throw failure;
}

export type Attempt<T> =
  | { ok: true; value: T }
  | { ok: false; code: ApiErrorKind };

export async function attempt<T>(
  next: string,
  fn: (clients: Clients) => Promise<T>,
): Promise<Attempt<T>> {
  const token = await requireSession(next);
  let failure: ReturnType<typeof toApiError>;
  try {
    return { ok: true, value: await fn(createGatewayClients({ token })) };
  } catch (err) {
    failure = toApiError(err);
  }
  if (failure.kind === "unauthenticated") redirect(refreshUrl(next));
  return { ok: false, code: failure.kind };
}

export const ERROR_MESSAGES: Record<string, string> = {
  invalid: "The request was not accepted. Check the details and try again.",
  forbidden: "You are not allowed to do that.",
  not_found: "That item no longer exists.",
  unavailable: "The service is busy. Try again in a moment.",
  internal: "Something went wrong. Try again.",
};

export function errorMessage(
  code: string | string[] | undefined,
): string | undefined {
  const key = Array.isArray(code) ? code[0] : code;
  return key ? (ERROR_MESSAGES[key] ?? ERROR_MESSAGES.internal) : undefined;
}
