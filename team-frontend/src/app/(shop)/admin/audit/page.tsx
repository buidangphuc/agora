import { redirectGuestToLogin } from "@/features/auth/guardPath";
import type { Metadata } from "next";

import { Result } from "@/components/ui/Result";
import { AuditLogView } from "@/features/admin/AuditLogView";
import {
  type SearchParams,
  parseAuditFilters,
} from "@/features/admin/auditFilters";
import { LinkButton } from "@/features/seller/LinkButton";
import { queryAuditLog } from "@/lib/gateway/audit";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Nhật ký kiểm toán | Sàn Thương Mại Điện Tử",
  description:
    "Tra cứu các thao tác quản trị: ai đã làm gì, lên đối tượng nào, vào lúc nào.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

function Forbidden({ subTitle }: { subTitle: string }) {
  return (
    <Result
      status="403"
      title="Cần tài khoản Quản trị"
      subTitle={subTitle}
      extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
    />
  );
}

// Same gate order as the cockpit page. The scope decode only gates rendering;
// the gateway enforces `admin` on the data itself.
export default async function AuditPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/audit");
    return null;
  }

  if (!principal.scopes.includes("admin")) {
    return (
      <Forbidden subTitle="Tài khoản của bạn không có quyền admin để xem Nhật ký kiểm toán." />
    );
  }

  const { filters, cursor, params, error } = parseAuditFilters(
    await searchParams,
  );
  if (error) {
    return (
      <AuditLogView events={[]} nextCursor="" params={params} error={error} />
    );
  }

  const result = await queryAuditLog(filters, cursor);
  if (result.status === "unauthenticated") {
    redirectGuestToLogin("/admin/audit");
    return null;
  }
  if (result.status === "forbidden") {
    return (
      <Forbidden subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này." />
    );
  }
  if (result.status === "ok") {
    return (
      <AuditLogView
        events={result.events}
        nextCursor={result.nextCursor}
        params={params}
      />
    );
  }
  // InvalidArgument without a date range is a rejected cursor, not a bad range.
  const rangeGiven = Boolean(filters.createdAfter || filters.createdBefore);
  const viewError =
    result.status === "invalid_range" && rangeGiven
      ? "invalid_range"
      : "unavailable";
  return (
    <AuditLogView events={[]} nextCursor="" params={params} error={viewError} />
  );
}
