import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Skeleton } from "@/components/ui/Skeleton";
import { formatPrice } from "@/components/ui/format";
import type { ViewOrderSettlement } from "@/lib/gateway/payment";

const TITLE = "Đối soát thanh toán";

export function OrderSettlementCardSkeleton() {
  return (
    <Card>
      <CardContent>
        <div
          className="h-44"
          aria-busy="true"
          data-testid="settlement-skeleton"
        >
          <Skeleton variant="text" lines={5} />
        </div>
      </CardContent>
    </Card>
  );
}

export function OrderSettlementCard({
  settlement,
  failed = false,
  retryHref,
}: {
  settlement: ViewOrderSettlement | null;
  failed?: boolean;
  retryHref: string;
}) {
  let body: React.ReactNode;
  if (failed) {
    body = (
      <Alert
        type="error"
        title="Không tải được đối soát thanh toán"
        action={<a href={retryHref}>Thử lại</a>}
      />
    );
  } else if (!settlement) {
    body = <p className="text-sm">Đơn chưa được đối soát vào ví</p>;
  } else {
    body = (
      <Descriptions
        column={1}
        items={[
          {
            key: "credited",
            label: "Số tiền được ghi có",
            children: formatPrice(settlement.credited),
          },
          {
            key: "commission",
            label: "Phí hoa hồng",
            children: `${formatPrice(settlement.commission)} (${settlement.commissionRatePercent}%)`,
          },
          {
            key: "reversed",
            label: "Hoàn phí hoa hồng",
            children: formatPrice(settlement.commissionReversed),
          },
          {
            key: "refund",
            label: "Hoàn tiền cho người mua",
            children: formatPrice(settlement.refundDeducted),
          },
          {
            key: "net",
            label: "Thực nhận",
            children: formatPrice(settlement.net),
          },
        ]}
      />
    );
  }
  return (
    <Card data-testid="order-settlement-card">
      <CardHeader>
        <CardTitle>{TITLE}</CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  );
}
