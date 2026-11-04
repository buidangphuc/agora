import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  type CampaignFilter,
  CampaignListView,
} from "@/features/campaigns/CampaignListView";
import {
  type CampaignFilterStatus,
  listCampaigns,
} from "@/lib/gateway/campaigns";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Chiến dịch flash sale | Sàn Thương Mại Điện Tử",
  description: "Xem, lọc, sửa và huỷ chiến dịch flash sale.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

type SearchParams = Record<string, string | string[] | undefined>;

const PATH = "/admin/campaigns";
const MAX_PARAM = 200;

function param(sp: SearchParams, key: string): string {
  const v = sp[key];
  const s = (Array.isArray(v) ? v[0] : v)?.trim() ?? "";
  return s.length > MAX_PARAM ? "" : s;
}

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

const STATUSES: CampaignFilterStatus[] = [
  "scheduled",
  "active",
  "ended",
  "cancelled",
];

export default async function CampaignsPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const sp = await searchParams;
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin(PATH, sp);
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return (
      <Forbidden subTitle="Tài khoản của bạn không có quyền admin để quản lý chiến dịch." />
    );
  }

  const cursor = param(sp, "cursor");
  const raw = param(sp, "status");
  const status = STATUSES.find((s) => s === raw);
  const filter: CampaignFilter = status ?? "all";
  const list = await listCampaigns(status ? [status] : [], cursor);
  if (list.status === "unauthenticated") {
    redirectGuestToLogin(PATH, sp);
    return null;
  }
  if (list.status === "forbidden") {
    return (
      <Forbidden subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này." />
    );
  }
  return (
    <CampaignListView
      campaigns={list.status === "ok" ? list.campaigns : []}
      nextCursor={list.status === "ok" ? list.nextCursor : ""}
      cursor={cursor}
      filter={filter}
      flashSaleDisabled={list.status === "ok" && list.flashSaleDisabled}
      error={list.status === "ok" ? undefined : "unavailable"}
    />
  );
}
