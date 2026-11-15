import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Table, type TableColumn } from "@/components/ui/Table";
import type {
  ViewDemandForecast,
  ViewForecastPoint,
} from "@/lib/gateway/analytics";
import { LinkButton } from "./LinkButton";

export interface ForecastItem {
  listingId: string;
  sku: string;
  result: PromiseSettledResult<ViewDemandForecast>;
}

const ERROR_TEXT = "Dự báo tạm thời không khả dụng";
const COLD_TEXT = "Chưa đủ dữ liệu để dự báo";

const one = (n: number) => n.toFixed(1);

const columns: TableColumn<ViewForecastPoint>[] = [
  { key: "date", title: "Ngày", dataIndex: "date" },
  { key: "p10", title: "P10", align: "right", render: (p) => one(p.p10) },
  { key: "p50", title: "P50", align: "right", render: (p) => one(p.p50) },
  { key: "p90", title: "P90", align: "right", render: (p) => one(p.p90) },
];

const runDate = (iso: string | null) => {
  if (!iso) return "";
  const d = new Date(new Date(iso).getTime() + 7 * 60 * 60 * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getUTCDate())}/${pad(d.getUTCMonth() + 1)}/${d.getUTCFullYear()}`;
};

function Body({
  item,
  retry,
}: {
  item: ForecastItem;
  retry: React.ReactNode;
}) {
  const { result } = item;
  if (result.status === "rejected") {
    return <Alert type="error" description={ERROR_TEXT} action={retry} />;
  }
  const f = result.value;
  if (f.isColdStart) return <Empty description={COLD_TEXT} />;
  if (f.dailyForecasts.length === 0) {
    return <Alert type="error" description={ERROR_TEXT} action={retry} />;
  }
  return (
    <div className="space-y-2">
      <Table
        caption={`Dự báo ${item.sku || item.listingId}`}
        columns={columns}
        dataSource={f.dailyForecasts}
        rowKey="date"
      />
      <p className="text-sm text-text-primary">
        {`Điểm đặt hàng lại: ${Math.ceil(f.suggestedReorderPoint)} đơn vị`}
      </p>
      <p className="text-xs text-text-secondary">
        {`Mô hình ${f.modelVersion} · chạy ngày ${runDate(f.generatedAt)}`}
      </p>
    </div>
  );
}

/** `items` null means the listing pick itself failed. */
export function ForecastCard({
  items,
  retryHref,
}: {
  items: ForecastItem[] | null;
  retryHref: string;
}) {
  const retry = (
    <LinkButton href={retryHref} size="sm">
      Thử lại
    </LinkButton>
  );
  return (
    <Card data-testid="forecast-card">
      <CardHeader>
        <CardTitle>Dự báo nhu cầu 14 ngày</CardTitle>
      </CardHeader>
      <CardContent>
        {items === null ? (
          <Alert type="error" description={ERROR_TEXT} action={retry} />
        ) : items.length === 0 ? (
          <Empty description="Chưa có đơn hàng để dự báo" />
        ) : (
          <div className="space-y-6">
            {items.map((item) => (
              <section
                key={item.listingId}
                data-testid="forecast-section"
                data-listing-id={item.listingId}
                className="space-y-2"
              >
                <h3 className="text-sm font-semibold text-text-primary">
                  {item.sku || item.listingId}
                </h3>
                <Body item={item} retry={retry} />
              </section>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
