import { createHandoffHandler } from "@fe-platform/session";

import { sellerOrigin, sellerSession } from "@/features/handoff/config";
import { peekHandoffCode, redeemHandoffCode } from "@/features/handoff/mint";

export const dynamic = "force-dynamic";

export const { GET, HEAD, POST } = createHandoffHandler(sellerSession, {
  origin: sellerOrigin(),
  peek: (code) => peekHandoffCode(code, "seller"),
  redeem: (code) => redeemHandoffCode(code, "seller"),
});
