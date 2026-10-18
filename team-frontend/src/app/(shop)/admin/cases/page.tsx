import type { Metadata } from "next";

import { Result } from "@/components/ui/Result";
import { AdminCasesTabs } from "@/features/admin/AdminCasesTabs";
import { ModerationQueueView } from "@/features/admin/ModerationQueueView";
import { ReportsView } from "@/features/admin/ReportsView";
import { parseCasesParams } from "@/features/admin/casesParams";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { LinkButton } from "@/features/seller/LinkButton";
import { listModerationQueue } from "@/lib/gateway/moderation";
import { listReports } from "@/lib/gateway/reports";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Hồ sơ cần xử lý | Sàn Thương Mại Điện Tử",
  description: "Báo cáo vi phạm và hàng chờ kiểm duyệt tin đăng.",
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

function Unavailable({
  tab,
  message,
}: {
  tab: "reports" | "queue";
  message: string;
}) {
  return (
    <>
      <AdminCasesTabs active={tab} />
      <div className="mx-auto max-w-6xl p-6">
        <p role="alert" className="text-sm text-text-secondary">
          {message}
        </p>
      </div>
    </>
  );
}

// The scope decode only gates rendering; the gateway enforces `admin` itself.
export default async function CasesPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const sp = await searchParams;
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/cases", sp);
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return (
      <Forbidden subTitle="Tài khoản của bạn không có quyền admin để xử lý hồ sơ." />
    );
  }

  const { tab, status, cursor } = parseCasesParams(sp);
  const denied = (
    <Forbidden subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này." />
  );

  if (tab === "queue") {
    const result = await listModerationQueue(cursor);
    if (result.status === "unauthenticated") {
      redirectGuestToLogin("/admin/cases", sp);
      return null;
    }
    if (result.status === "forbidden") return denied;
    if (result.status !== "ok") {
      return (
        <Unavailable
          tab="queue"
          message="Không tải được hàng chờ kiểm duyệt. Vui lòng thử lại sau."
        />
      );
    }
    return (
      <>
        <AdminCasesTabs active="queue" />
        <ModerationQueueView
          items={result.items}
          nextCursor={result.nextCursor}
          basePath="/admin/cases"
          tab="queue"
        />
      </>
    );
  }

  const result = await listReports(
    status.toLowerCase() as "open" | "actioned" | "dismissed",
    cursor,
  );
  if (result.status === "unauthenticated") {
    redirectGuestToLogin("/admin/cases", sp);
    return null;
  }
  if (result.status === "forbidden") return denied;
  if (result.status !== "ok") {
    return (
      <Unavailable
        tab="reports"
        message="Không tải được danh sách báo cáo. Vui lòng thử lại sau."
      />
    );
  }
  return (
    <>
      <AdminCasesTabs active="reports" />
      <ReportsView
        reports={result.items}
        nextCursor={result.nextCursor}
        status={status}
      />
    </>
  );
}
