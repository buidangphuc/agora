import { Alert } from "@/components/ui/Alert";
import type { ViewModeration } from "@/lib/gateway/listings";

import { reasonLabel } from "./reasonLabels";

const HELD_TEXT =
  "Tin đăng đang tạm ẩn vì tài khoản của bạn bị hạn chế. Tin sẽ hiển thị lại khi tài khoản được khôi phục.";

function decision(prefix: string, m?: ViewModeration): string {
  const reason = m?.reasonCode ? reasonLabel(m.reasonCode) : "";
  const note = m?.note?.trim() ?? "";
  return [reason ? `${prefix}: ${reason}.` : `${prefix}.`, note]
    .filter(Boolean)
    .join(" ");
}

/** Seller-facing moderation outcome; never renders screen flags. */
export function ModerationBanner({
  status,
  moderation,
}: { status: string; moderation?: ViewModeration }) {
  switch (status) {
    case "rejected":
      return (
        <Alert
          type="error"
          description={decision("Tin đăng bị từ chối", moderation)}
        />
      );
    case "banned":
      return (
        <Alert
          type="error"
          description={decision("Tin đăng bị khóa", moderation)}
        />
      );
    case "held":
      return <Alert type="warning" description={HELD_TEXT} />;
    case "pending_review":
      return <Alert type="info" description="Tin đăng đang chờ kiểm duyệt." />;
    default:
      return null;
  }
}
