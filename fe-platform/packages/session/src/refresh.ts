import { createGatewayClients } from "@fe-platform/api-client";
import type { RefreshResult } from "./middleware";

export function gatewayRefresh(opts: { gatewayUrl?: string } = {}) {
  return async function refresh(refreshToken: string): Promise<RefreshResult> {
    const clients = createGatewayClients({ baseUrl: opts.gatewayUrl });
    const res = await clients.auth.refreshSession({ refreshToken });
    return {
      token: res.result?.token ?? "",
      refreshToken: res.result?.refreshToken || undefined,
    };
  };
}
