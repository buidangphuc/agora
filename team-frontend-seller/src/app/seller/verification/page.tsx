import { Alert } from "@/components/ui/Alert";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { LinkButton } from "@/features/seller/LinkButton";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import type { SearchParams } from "@/features/seller/listParams";
import { VerificationPanel } from "@/features/seller/verification/VerificationPanel";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import {
  type ViewVerificationStatus,
  getVerificationStatus,
} from "@/lib/gateway/verification";

export const dynamic = "force-dynamic";

export const metadata = { title: "Xác minh người bán" };

export default async function SellerVerificationPage(props: {
  searchParams?: Promise<SearchParams>;
}) {
  const searchParams = await props.searchParams;
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/seller/verification", searchParams);
  if (!hasScope("listing.write")) return null;

  let view: ViewVerificationStatus | null = null;
  try {
    view = await getVerificationStatus(me.id);
  } catch {
    view = null;
  }

  return (
    <>
      <SellerPageHeader
        title="Xác minh người bán"
        description="Gửi giấy tờ để xác minh danh tính và nhận huy hiệu."
      />
      {view === null ? (
        <Alert
          type="error"
          description="Không tải được trạng thái xác minh."
          action={
            <LinkButton href="/seller/verification" size="sm">
              Thử lại
            </LinkButton>
          }
        />
      ) : (
        <VerificationPanel
          status={view.status}
          rejectReasonCode={view.rejectReasonCode}
        />
      )}
    </>
  );
}
