import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { PriceTag } from "@/components/ui/PriceTag";
import { Progress } from "@/components/ui/Progress";
import { Table, type TableColumn } from "@/components/ui/Table";
import { formatPrice } from "@/components/ui/format";
import type {
  ViewDayRevenue,
  ViewRevenueBreakdown,
  ViewSellerFunnel,
  ViewTopSku,
} from "@/lib/gateway/analytics";
import { KpiRow } from "./KpiRow";
import { LinkButton } from "./LinkButton";
import { type Range, daysInRange, ratio } from "./analyticsRange";

const num = (n: number) => n.toLocaleString("vi-VN");

const dayColumns: TableColumn<ViewDayRevenue>[] = [
  { key: "day", title: "Ngày", dataIndex: "day" },
  {
    key: "orders",
    title: "Đơn hàng",
    align: "right",
    render: (d) => num(d.orderCount),
  },
  {
    key: "revenue",
    title: "Doanh thu",
    align: "right",
    render: (d) => <PriceTag price={d.revenue} size="md" />,
  },
];

const skuColumns: TableColumn<ViewTopSku>[] = [
  { key: "sku", title: "SKU", render: (t) => t.sku || t.listingId },
  {
    key: "units",
    title: "Đã bán",
    align: "right",
    render: (t) => num(t.unitsSold),
  },
  {
    key: "revenue",
    title: "Doanh thu",
    align: "right",
    render: (t) => <PriceTag price={t.revenue} size="md" />,
  },
];

const FUNNEL_STEPS: {
  key: keyof ViewSellerFunnel;
  label: string;
}[] = [
  { key: "impressions", label: "Lượt hiển thị" },
  { key: "views", label: "Lượt xem" },
  { key: "adds", label: "Thêm giỏ" },
  { key: "orders", label: "Đơn hàng" },
];

/**
 * Dashboard > Analysis body. `funnel` / `revenue` are null when their call
 * failed: that block shows an Alert with a retry link and the rest renders.
 */
export function AnalyticsView({
  funnel,
  revenue,
  range,
  retryHref,
  now,
}: {
  funnel: ViewSellerFunnel | null;
  revenue: ViewRevenueBreakdown | null;
  range: Range;
  retryHref: string;
  now?: number;
}) {
  const days = revenue ? daysInRange(revenue.days, range, now) : [];
  const totalRevenue = days.reduce((s, d) => s + d.revenue, 0);
  const totalOrders = days.reduce((s, d) => s + d.orderCount, 0);

  const cells = [
    ...(revenue
      ? [
          {
            key: "revenue",
            title: "Doanh thu",
            value: formatPrice(totalRevenue),
          },
          { key: "orders", title: "Đơn hàng", value: num(totalOrders) },
        ]
      : []),
    ...(funnel
      ? [
          {
            key: "impressions",
            title: "Lượt hiển thị",
            value: num(funnel.impressions),
          },
          ...(funnel.impressions > 0
            ? [
                {
                  key: "conversion",
                  title: "Tỷ lệ chuyển đổi",
                  value: `${((funnel.orders / funnel.impressions) * 100).toFixed(1)}%`,
                },
              ]
            : []),
        ]
      : []),
  ];

  const retry = (
    <LinkButton href={retryHref} size="sm">
      Thử lại
    </LinkButton>
  );

  return (
    <>
      {cells.length > 0 && <KpiRow cells={cells} />}

      <Card>
        <CardHeader>
          <CardTitle>Phễu chuyển đổi</CardTitle>
          <span className="text-xs text-text-secondary">Toàn thời gian</span>
        </CardHeader>
        <CardContent className="space-y-4">
          {funnel === null ? (
            <Alert
              type="error"
              description="Không tải được dữ liệu phễu chuyển đổi."
              action={retry}
            />
          ) : funnel.impressions === 0 && funnel.views === 0 ? (
            <Empty description="Chưa có dữ liệu phễu" />
          ) : (
            FUNNEL_STEPS.map((step) => (
              <div key={step.key} className="space-y-1">
                <div className="flex items-baseline justify-between text-sm">
                  <span className="text-text-secondary">{step.label}</span>
                  <span className="font-semibold text-text-primary">
                    {num(funnel[step.key])}
                  </span>
                </div>
                <Progress
                  percent={ratio(funnel[step.key], funnel.impressions)}
                  label={step.label}
                />
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Doanh thu theo ngày</CardTitle>
        </CardHeader>
        <CardContent>
          {revenue === null ? (
            <Alert
              type="error"
              description="Không tải được dữ liệu doanh thu."
              action={retry}
            />
          ) : (
            <Table
              caption="Doanh thu theo ngày"
              columns={dayColumns}
              dataSource={days}
              rowKey="day"
              emptyText="Chưa có doanh thu trong khoảng này"
            />
          )}
        </CardContent>
      </Card>

      {revenue && revenue.topSkus.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>SKU doanh thu cao nhất</CardTitle>
          </CardHeader>
          <CardContent>
            <Table
              caption="SKU doanh thu cao nhất"
              columns={skuColumns}
              dataSource={revenue.topSkus.slice(0, 5)}
              rowKey={(t) => t.listingId || t.sku}
            />
          </CardContent>
        </Card>
      )}
    </>
  );
}
