import type { Tone } from "@/components/ui/tones";
import { VerificationStatus } from "@/lib/gateway/verification";

/** KYC status to the semantic Tag tone: success, promo (warning), danger, neutral. Never brand. */
export function statusTone(status: VerificationStatus): Tone {
  switch (status) {
    case VerificationStatus.VERIFIED:
      return "success";
    case VerificationStatus.PENDING:
      return "warning";
    case VerificationStatus.REJECTED:
      return "danger";
    default:
      return "neutral";
  }
}
