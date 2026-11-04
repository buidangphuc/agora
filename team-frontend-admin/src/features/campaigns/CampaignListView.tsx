import { Alert } from "@/components/ui/Alert";
import { Card } from "@/components/ui/Card";
import { Link } from "@/components/ui/Link";
import type {
  CampaignFilterStatus,
  CampaignView,
} from "@/lib/gateway/campaigns";

import { CampaignRow } from "./CampaignRow";

export type CampaignFilter = "all" | CampaignFilterStatus;

const FILTERS: { key: CampaignFilter; label: string }[] = [
  { key: "all", label: "Tất cả" },
  { key: "scheduled", label: "Sắp chạy" },
  { key: "active", label: "Đang chạy" },
  { key: "ended", label: "Đã kết thúc" },
  { key: "cancelled", label: "Đã huỷ" },
];

function href(filter: CampaignFilter, cursor: string): string {
  const q = new URLSearchParams();
  if (filter !== "all") q.set("status", filter);
  if (cursor) q.set("cursor", cursor);
  const qs = q.toString();
  return qs ? `/admin/campaigns?${qs}` : "/admin/campaigns";
}

export function CampaignListView({
  campaigns,
  nextCursor,
  cursor,
  filter,
  flashSaleDisabled,
  error,
}: {
  campaigns: CampaignView[];
  nextCursor: string;
  cursor: string;
  filter: CampaignFilter;
  flashSaleDisabled: boolean;
  error?: "unavailable";
}) {
  return (
    <div className="flex flex-col gap-6 md:gap-8">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div className="flex min-w-0 flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
            Chiến dịch flash sale
          </h1>
          <p className="text-base text-text-secondary">
            Quản lý chiến dịch flash sale trên toàn sàn.
          </p>
        </div>
        <Link
          href="/admin/campaigns/new"
          variant="primary"
          size="md"
          fullWidth="mobile"
          data-testid="campaign-create-link"
        >
          Tạo flash sale
        </Link>
      </header>

      {flashSaleDisabled ? (
        <Alert
          type="warning"
          data-testid="flash-sale-disabled-banner"
          description="Flash sale is switched off. Chiến dịch vẫn được lưu nhưng người mua chưa thấy giá flash sale."
        />
      ) : null}

      <nav aria-label="Lọc theo trạng thái" className="flex flex-wrap gap-2">
        {FILTERS.map((f) => (
          <Link
            key={f.key}
            href={href(f.key, "")}
            variant={f.key === filter ? "primary" : "outline"}
            size="sm"
            data-testid={`campaign-filter-${f.key}`}
            aria-current={f.key === filter ? "true" : undefined}
          >
            {f.label}
          </Link>
        ))}
      </nav>

      {error ? (
        <Alert
          type="error"
          title="Không tải được danh sách chiến dịch"
          description="Dịch vụ khuyến mãi đang tạm ngưng hoặc không phản hồi. Vui lòng tải lại trang sau."
        />
      ) : campaigns.length === 0 ? (
        flashSaleDisabled ? null : (
          <Alert
            type="info"
            data-testid="campaigns-empty"
            description="Không có chiến dịch nào."
          />
        )
      ) : (
        <Card>
          <table className="w-full text-left text-sm">
            <caption className="sr-only">Chiến dịch flash sale</caption>
            <thead>
              <tr>
                <th scope="col">Mã chiến dịch</th>
                <th scope="col">Mã tin đăng</th>
                <th scope="col">Giá flash sale</th>
                <th scope="col">Đã bán / tối đa</th>
                <th scope="col">Thời gian</th>
                <th scope="col">Trạng thái</th>
                <th scope="col">Thao tác</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((c) => (
                <CampaignRow key={c.id} campaign={c} />
              ))}
            </tbody>
          </table>
        </Card>
      )}

      {cursor || nextCursor ? (
        <div className="flex gap-2">
          {cursor ? (
            <Link
              href={href(filter, "")}
              variant="outline"
              size="sm"
              data-testid="campaigns-first"
            >
              Trang đầu
            </Link>
          ) : null}
          {nextCursor ? (
            <Link
              href={href(filter, nextCursor)}
              variant="outline"
              size="sm"
              data-testid="campaigns-next"
            >
              Trang sau
            </Link>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
