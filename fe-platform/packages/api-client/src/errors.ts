import { Code, ConnectError } from "@connectrpc/connect";

export type ApiErrorKind =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "invalid"
  | "unavailable"
  | "internal";

const KIND_BY_CODE: Record<number, ApiErrorKind> = {
  [Code.Unauthenticated]: "unauthenticated",
  [Code.PermissionDenied]: "forbidden",
  [Code.NotFound]: "not_found",
  [Code.InvalidArgument]: "invalid",
  [Code.FailedPrecondition]: "invalid",
  [Code.OutOfRange]: "invalid",
  [Code.AlreadyExists]: "invalid",
  [Code.Unavailable]: "unavailable",
  [Code.DeadlineExceeded]: "unavailable",
  [Code.ResourceExhausted]: "unavailable",
};

export class ApiError extends Error {
  readonly kind: ApiErrorKind;
  readonly code: Code | undefined;
  readonly requestId: string | undefined;

  constructor(
    kind: ApiErrorKind,
    message: string,
    code?: Code,
    requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
    this.kind = kind;
    this.code = code;
    this.requestId = requestId;
  }
}

export function toApiError(err: unknown, requestId?: string): ApiError {
  if (err instanceof ApiError) return err;
  if (err instanceof ConnectError) {
    return new ApiError(
      KIND_BY_CODE[err.code] ?? "internal",
      err.rawMessage,
      err.code,
      requestId,
    );
  }
  const message = err instanceof Error ? err.message : "unknown error";
  return new ApiError("internal", message, undefined, requestId);
}
