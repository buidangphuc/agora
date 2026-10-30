import type { Metadata } from "next";
import Link from "next/link";

import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";
import { AdminCasesTabs } from "@/features/admin/AdminCasesTabs";
import {
  DisputeCasesPanel,
  type DisputeFilter,
} from "@/features/admin/DisputeCasesPanel";
import { ModerationQueueView } from "@/features/admin/ModerationQueueView";
import { ReportsView } from "@/features/admin/ReportsView";
import { parseCasesParams } from "@/features/admin/casesParams";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getDispute, listDisputes } from "@/lib/gateway/disputes";
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

const DISPUTE_FILTERS: DisputeFilter[] = [
  "open",
  "investigating",
  "resolved",
  "rejected",
];

function first(v: string | string[] | undefined): string {
  const s = Array.isArray(v) ? v[0] : v;
  return typeof s === "string" ? s.trim() : "";
}

function DisputesTabs() {
  const tabs = [
    { label: "Báo cáo", href: "/admin/cases?tab=reports", active: false },
    { label: "Hàng chờ duyệt", href: "/admin/cases?tab=queue", active: false },
    { label: "Khiếu nại", href: "/admin/cases?tab=disputes", active: true },
  ];
  return (
    <nav aria-label="Hồ sơ cần xử lý" className="flex gap-4 border-b">
      {tabs.map((t) => (
        <Link
          key={t.label}
          href={t.href}
          aria-current={t.active ? "page" : undefined}
          className={`px-2 py-2 text-sm ${t.active ? "border-b-2 font-semibold text-text-primary" : "text-text-secondary"}`}
        >
          {t.label}
        </Link>
      ))}
    </nav>
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

  if (first(sp.tab) === "disputes") {
    const rawStatus = first(sp.status).toLowerCase() as DisputeFilter;
    const filter = DISPUTE_FILTERS.includes(rawStatus) ? rawStatus : "open";
    const disputeId = first(sp.dispute);
    const list = await listDisputes(filter.toUpperCase(), cursor);
    if (list.status === "unauthenticated") {
      redirectGuestToLogin("/admin/cases", sp);
      return null;
    }
    if (list.status === "forbidden") return denied;
    if (list.status !== "ok") {
      return (
        <>
          <DisputesTabs />
          <div className="mx-auto max-w-6xl p-6">
            <p role="alert" className="text-sm text-text-secondary">
              Không tải được danh sách khiếu nại. Vui lòng thử lại sau.
            </p>
          </div>
        </>
      );
    }
    let detail: Awaited<ReturnType<typeof getDispute>> | undefined;
    if (disputeId) {
      detail = await getDispute(disputeId);
      if (detail.status === "unauthenticated") {
        redirectGuestToLogin("/admin/cases", sp);
        return null;
      }
      if (detail.status === "forbidden") return denied;
    }
    return (
      <>
        <DisputesTabs />
        {detail && detail.status === "not_found" ? (
          <p role="alert" className="mx-auto max-w-6xl p-6 pb-0 text-sm">
            Không tìm thấy khiếu nại này.
          </p>
        ) : null}
        {detail && detail.status === "unavailable" ? (
          <p role="alert" className="mx-auto max-w-6xl p-6 pb-0 text-sm">
            Không tải được khiếu nại này. Vui lòng thử lại sau.
          </p>
        ) : null}
        <DisputeCasesPanel
          disputes={list.disputes}
          nextCursor={list.nextCursor}
          status={filter}
          detail={detail && detail.status === "ok" ? detail.dispute : undefined}
        />
      </>
    );
  }

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
