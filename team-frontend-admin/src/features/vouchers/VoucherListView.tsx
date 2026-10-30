import { LinkButton } from "@/components/LinkButton";
import { Alert } from "@/components/ui/Alert";
import { Link } from "@/components/ui/Link";
import { Table } from "@/components/ui/Table";
import type { VoucherView } from "@/lib/gateway/vouchers";

import { VoucherStateBadge } from "./VoucherDetails";
import {
  describeDiscount,
  describeMinSpend,
  describeUsage,
  formatVnDateTime,
  voucherHref,
} from "./voucherFormat";

export function VoucherListView({
  vouchers,
  nextCursor,
  cursor,
  now,
  error,
}: {
  vouchers: VoucherView[];
  nextCursor: string;
  cursor: string;
  now: Date;
  error?: "unavailable";
}) {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-text-primary">Voucher sàn</h1>
          <p className="text-sm text-text-secondary">
            Voucher áp dụng cho mọi người mua trên toàn sàn.
          </p>
        </div>
        <LinkButton href="/admin/vouchers/new" variant="primary">
          Tạo voucher sàn
        </LinkButton>
      </div>

      {error ? (
        <Alert
          type="error"
          title="Không tải được danh sách voucher"
          description="Dịch vụ khuyến mãi đang tạm ngưng hoặc không phản hồi. Vui lòng tải lại trang sau."
        />
      ) : (
        <Table<VoucherView>
          caption="Danh sách voucher sàn"
          dataSource={vouchers}
          rowKey="id"
          emptyText="Chưa có voucher sàn nào"
          rowProps={() => ({ "data-testid": "voucher-row" })}
          columns={[
            {
              key: "code",
              title: "Mã",
              render: (v) => <Link href={voucherHref(v.code)}>{v.code}</Link>,
            },
            {
              key: "discount",
              title: "Mức giảm",
              render: (v) => describeDiscount(v),
            },
            {
              key: "minSpend",
              title: "Đơn tối thiểu",
              hideBelow: "md",
              render: (v) => describeMinSpend(v.minSpend),
            },
            {
              key: "usage",
              title: "Đã dùng",
              hideBelow: "sm",
              render: (v) => describeUsage(v),
            },
            {
              key: "endsAt",
              title: "Kết thúc",
              hideBelow: "md",
              render: (v) => formatVnDateTime(v.endsAt),
            },
            {
              key: "state",
              title: "Trạng thái",
              render: (v) => <VoucherStateBadge voucher={v} now={now} />,
            },
          ]}
        />
      )}

      <div className="flex gap-4 text-sm">
        {cursor ? (
          <Link href="/admin/vouchers" variant="subtle">
            Trang đầu
          </Link>
        ) : null}
        {nextCursor ? (
          <Link
            href={`/admin/vouchers?cursor=${encodeURIComponent(nextCursor)}`}
            variant="subtle"
          >
            Trang sau
          </Link>
        ) : null}
      </div>
    </div>
  );
}
