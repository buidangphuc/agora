import { createHandoffHandler } from "@fe-platform/session";

import { buyerHandoffSession, buyerOrigin } from "@/features/handoff/config";
import { peekHandoffCode, redeemHandoffCode } from "@/features/handoff/mint";

export const dynamic = "force-dynamic";

export const { GET, HEAD, POST } = createHandoffHandler(buyerHandoffSession, {
  origin: buyerOrigin(),
  peek: (code) => peekHandoffCode(code, "buyer"),
  redeem: (code) => redeemHandoffCode(code, "buyer"),
});
