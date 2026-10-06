import { ConnectError } from "@connectrpc/connect";

/**
 * User-readable message for a failed gateway call inside a Server Action:
 * the Connect error's own message, else the Error message, else `fallback`.
 * Actions turn it into `fail(message)` so they resolve instead of throwing.
 */
export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof ConnectError) return err.rawMessage || fallback;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}
