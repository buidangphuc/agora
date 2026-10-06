import { LoyaltyWidget } from "@/features/engagement/LoyaltyWidget";
import { getLoyalty } from "@/lib/gateway/engagement";
import { getPrincipal } from "@/lib/gateway/session";

/** Daily check-in widget for signed-in buyers only. */
export async function LoyaltyBlock() {
  if (!getPrincipal()) return null;
  const loyalty = await getLoyalty();
  return <LoyaltyWidget initial={loyalty} />;
}
