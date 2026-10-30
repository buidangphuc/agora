"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
import { Table } from "@/components/ui/Table";
import { formatPrice } from "@/components/ui/format";
import type { CampaignView } from "@/lib/gateway/campaigns";

import { CampaignDetails } from "./CampaignDetails";
import { formatCampaignTime } from "./campaignForm";

export type CampaignLookup =
  | { listingId: string; status: "active"; campaign: CampaignView }
  | {
      listingId: string;
      status: "none" | "invalid" | "unavailable";
    };

function nextHref(cursor: string, listingId: string): string {
  const q = new URLSearchParams({ cursor });
  if (listingId) q.set("listing", listingId);
  return `/admin/campaigns?${q.toString()}`;
}

function LookupResult({ lookup }: { lookup: CampaignLookup }) {
  if (lookup.status === "active") {
    return (
      <Card padded>
        <CampaignDetails
          campaign={lookup.campaign}
          title={`Flash sale đang chạy của tin ${lookup.listingId}`}
          testId="campaign-lookup-result"
        />
      </Card>
    );
  }
  if (lookup.status === "none") {
    return (
      <Alert
        type="info"
        data-testid="campaign-lookup-result"
        description={`Tin ${lookup.listingId} không có flash sale nào đang chạy.`}
      />
    );
  }
  return (
    <Alert
      type="error"
      data-testid="campaign-lookup-result"
      description={
        lookup.status === "invalid"
          ? "Mã tin đăng không hợp lệ."
          : "Không tra cứu được flash sale. Vui lòng thử lại sau."
      }
    />
  );
}

export function CampaignsView({
  campaigns,
  nextCursor,
  lookup,
  error,
}: {
  campaigns: CampaignView[];
  nextCursor: string;
  lookup: CampaignLookup | null;
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
            Flash sale đang chạy trên toàn sàn và tra cứu theo tin đăng.
          </p>
        </div>
        <Link
          href="/admin/campaigns/new"
          variant="primary"
          size="md"
          fullWidth="mobile"
        >
          Tạo flash sale
        </Link>
      </header>

      <Alert
        type="info"
        data-testid="campaigns-gap"
        description="Danh sách chỉ gồm chiến dịch đang chạy. Hệ thống chưa có chức năng xem chiến dịch sắp chạy hoặc đã kết thúc, và chưa thể huỷ hay sửa chiến dịch."
      />

      <Card padded>
        <form
          method="get"
          action="/admin/campaigns"
          data-testid="campaign-lookup"
          className="flex flex-col gap-4 sm:flex-row sm:items-end"
        >
          <div className="min-w-0 sm:flex-1 md:max-w-md">
            <Input
              name="listing"
              label="Tra cứu theo mã tin đăng"
              defaultValue={lookup?.listingId ?? ""}
              maxLength={200}
              required
            />
          </div>
          <Button type="submit" variant="secondary" fullWidth="mobile">
            Tra cứu
          </Button>
        </form>
      </Card>

      {lookup ? <LookupResult lookup={lookup} /> : null}

      <section
        aria-labelledby="campaigns-active-heading"
        className="flex flex-col gap-4"
      >
        <h2
          id="campaigns-active-heading"
          className="text-xl font-semibold text-text-primary"
        >
          Đang chạy
        </h2>
        <Card>
          <Table<CampaignView>
            caption="Chiến dịch flash sale đang chạy"
            rowKey="id"
            dataSource={campaigns}
            error={
              error
                ? "Không tải được danh sách chiến dịch. Vui lòng thử lại sau."
                : undefined
            }
            emptyText="Không có chiến dịch nào đang chạy."
            rowProps={() => ({ "data-testid": "campaigns-row" })}
            columns={[
              { key: "id", title: "Mã chiến dịch", dataIndex: "id" },
              { key: "listing", title: "Mã tin đăng", dataIndex: "listingId" },
              {
                key: "variant",
                title: "Phân loại",
                hideBelow: "md",
                render: (c) => c.variantId || "Tất cả",
              },
              {
                key: "price",
                title: "Giá flash sale",
                align: "right",
                render: (c) => formatPrice(c.salePrice),
              },
              {
                key: "stock",
                title: "Đã bán / tối đa",
                align: "right",
                render: (c) => `${c.stockSold} / ${c.stockCap}`,
              },
              {
                key: "window",
                title: "Thời gian",
                hideBelow: "sm",
                render: (c) =>
                  `${formatCampaignTime(c.startsAt)} – ${formatCampaignTime(c.endsAt)}`,
              },
            ]}
          />
        </Card>
        {nextCursor ? (
          <div className="flex">
            <Link
              href={nextHref(nextCursor, lookup?.listingId ?? "")}
              variant="outline"
              size="sm"
            >
              Trang sau
            </Link>
          </div>
        ) : null}
      </section>
    </div>
  );
}
