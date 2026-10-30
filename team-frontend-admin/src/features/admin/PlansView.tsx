import { Alert } from "@/components/ui/Alert";
import { Table } from "@/components/ui/Table";
import { formatPrice } from "@/components/ui/format";
import type { PlanTierName, PlanView } from "@/lib/gateway/campaigns";

const TIER_LABEL: Record<PlanTierName, string> = {
  free: "Miễn phí",
  pro: "Pro",
  premium: "Premium",
  unspecified: "Không rõ",
};

export function PlansView({
  plans,
  error,
}: {
  plans: PlanView[];
  error?: "unavailable";
}) {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-text-primary">
        Gói đăng ký người bán
      </h1>
      <Alert
        type="info"
        data-testid="plans-gap"
        description="Chỉ xem. Danh mục gói do dịch vụ khuyến mãi quản lý; hệ thống chưa có chức năng tạo hay sửa gói."
      />
      <Table<PlanView>
        caption="Gói đăng ký người bán"
        rowKey="id"
        dataSource={plans}
        error={
          error
            ? "Không tải được danh sách gói đăng ký. Vui lòng thử lại sau."
            : undefined
        }
        emptyText="Chưa có gói đăng ký nào."
        rowProps={() => ({ "data-testid": "plans-row" })}
        columns={[
          {
            key: "tier",
            title: "Gói",
            render: (p) => TIER_LABEL[p.tier],
          },
          {
            key: "price",
            title: "Giá mỗi kỳ",
            align: "right",
            render: (p) => formatPrice(p.price),
          },
          {
            key: "features",
            title: "Quyền lợi",
            render: (p) =>
              p.features.length > 0 ? (
                <ul className="list-disc pl-4">
                  {p.features.map((f) => (
                    <li key={f}>{f}</li>
                  ))}
                </ul>
              ) : (
                "Không có"
              ),
          },
          { key: "id", title: "Mã gói", dataIndex: "id", hideBelow: "md" },
        ]}
      />
    </div>
  );
}
