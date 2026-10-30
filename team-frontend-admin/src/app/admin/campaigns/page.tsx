import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";
import {
  type CampaignLookup,
  CampaignsView,
} from "@/features/admin/CampaignsView";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  getActiveFlashSale,
  listActiveCampaigns,
} from "@/lib/gateway/campaigns";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Chiến dịch flash sale | Sàn Thương Mại Điện Tử",
  description: "Xem chiến dịch flash sale đang chạy và tra cứu theo tin đăng.",
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

async function lookupFor(listingId: string): Promise<CampaignLookup | null> {
  if (!listingId) return null;
  const res = await getActiveFlashSale(listingId);
  if (res.status !== "ok") {
    return {
      listingId,
      status: res.status === "invalid" ? "invalid" : "unavailable",
    };
  }
  return res.active
    ? { listingId, status: "active", campaign: res.campaign }
    : { listingId, status: "none" };
}

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
  const listingId = param(sp, "listing");
  const [list, lookup] = await Promise.all([
    listActiveCampaigns(cursor),
    lookupFor(listingId),
  ]);
  if (list.status === "unauthenticated") {
    redirectGuestToLogin(PATH, sp);
    return null;
  }
  if (list.status === "forbidden") {
    return (
      <Forbidden subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này." />
    );
  }
  if (list.status === "ok") {
    return (
      <CampaignsView
        campaigns={list.campaigns}
        nextCursor={list.nextCursor}
        lookup={lookup}
      />
    );
  }
  return (
    <CampaignsView
      campaigns={[]}
      nextCursor=""
      lookup={lookup}
      error="unavailable"
    />
  );
}
