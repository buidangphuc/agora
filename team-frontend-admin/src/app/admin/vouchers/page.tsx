import type { Metadata } from "next";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  GATEWAY_DENIED,
  NO_ADMIN_SCOPE,
  VoucherForbidden,
} from "@/features/vouchers/VoucherForbidden";
import { VoucherListView } from "@/features/vouchers/VoucherListView";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import { listPlatformVouchers } from "@/lib/gateway/vouchers";

export const metadata: Metadata = {
  title: "Voucher sàn | Sàn Thương Mại Điện Tử",
  description: "Danh sách voucher áp dụng cho toàn sàn.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

type SearchParams = Record<string, string | string[] | undefined>;

const MAX_CURSOR = 512;

function cursorFrom(sp: SearchParams): string {
  const raw = sp.cursor;
  const v = Array.isArray(raw) ? raw[0] : raw;
  return v && v.length <= MAX_CURSOR ? v : "";
}

export default async function VouchersPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const sp = await searchParams;
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/vouchers", sp);
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return <VoucherForbidden subTitle={NO_ADMIN_SCOPE} />;
  }

  const cursor = cursorFrom(sp);
  const result = await listPlatformVouchers(cursor);
  const now = new Date();
  if (result.status === "unauthenticated") {
    redirectGuestToLogin("/admin/vouchers", sp);
    return null;
  }
  if (result.status === "forbidden") {
    return <VoucherForbidden subTitle={GATEWAY_DENIED} />;
  }
  if (result.status === "ok") {
    return (
      <VoucherListView
        vouchers={result.vouchers}
        nextCursor={result.nextCursor}
        cursor={cursor}
        now={now}
      />
    );
  }
  return (
    <VoucherListView
      vouchers={[]}
      nextCursor=""
      cursor={cursor}
      now={now}
      error="unavailable"
    />
  );
}
