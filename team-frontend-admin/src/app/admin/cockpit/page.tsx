import { redirectGuestToLogin } from "@/features/auth/guardPath";
import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";
import { CockpitView } from "@/features/admin/CockpitView";
import { InsightsSection } from "@/features/admin/insights/InsightsSection";
import { fetchCockpit } from "@/lib/gateway/cockpit";
import { fetchInsights, parseInsightParams } from "@/lib/gateway/insights";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Admin Operations Cockpit",
  description:
    "Bản đồ vận hành thời gian thực, telemetry, Golden Signals và Jaeger Distributed Tracing.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

// Session check and first fetch both happen server-side. The scope decode is for
// gating the render only; the gateway enforces `admin` on the data itself.
type SearchParams = Record<string, string | string[] | undefined>;

export default function CockpitPage(): Promise<React.ReactNode>;
export default function CockpitPage(props: {
  searchParams: SearchParams;
}): Promise<React.ReactNode>;
export default function CockpitPage(props: {
  searchParams?: Promise<SearchParams>;
}): Promise<React.ReactNode>;
export default async function CockpitPage(props?: {
  searchParams?: SearchParams | Promise<SearchParams>;
}): Promise<React.ReactNode> {
  const searchParams: SearchParams = (await props?.searchParams) ?? {};
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/cockpit");
    return null;
  }

  if (!principal.scopes.includes("admin")) {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Tài khoản của bạn không có quyền admin để xem Operations Cockpit."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }

  const result = await fetchCockpit(token);
  if (result.status === "unauthenticated") {
    redirectGuestToLogin("/admin/cockpit");
    return null;
  }
  if (result.status === "forbidden") {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }
  const params = parseInsightParams(searchParams);
  const insights = await fetchInsights(params);
  const reports = [
    insights.recommendation,
    insights.search,
    insights.retention,
    insights.experiment,
    insights.aiSurfaces,
  ];
  if (reports.some((r) => r.status === "unauthenticated")) {
    redirectGuestToLogin("/admin/cockpit");
    return null;
  }
  if (reports.some((r) => r.status === "forbidden")) {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }
  const rawFlag = searchParams.flag;
  const flagText = (Array.isArray(rawFlag) ? rawFlag[0] : rawFlag) ?? "";
  return (
    <>
      <CockpitView initial={result.status === "ok" ? result.data : null} />
      <InsightsSection
        insights={insights}
        params={params}
        flagText={flagText}
      />
    </>
  );
}
