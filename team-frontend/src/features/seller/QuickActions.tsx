import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { LinkButton } from "./LinkButton";

const ACTIONS = [
  { href: "/seller/new", label: "Thêm sản phẩm" },
  { href: "/seller/orders", label: "Quản lý đơn hàng" },
  { href: "/seller/wallet", label: "Ví người bán" },
  { href: "/seller/ads", label: "Quảng cáo" },
  { href: "/seller/bundles", label: "Combo sản phẩm" },
];

/** Workplace quick actions: outline links (the one brand CTA is the page header's). */
export function QuickActions() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Thao tác nhanh</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-3">
        {ACTIONS.map((a) => (
          <LinkButton key={a.href} href={a.href} variant="outline">
            {a.label}
          </LinkButton>
        ))}
      </CardContent>
    </Card>
  );
}
