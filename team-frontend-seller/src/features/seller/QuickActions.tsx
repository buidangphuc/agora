import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/Card";
import { Link } from "@/components/ui/Link";
import { type IconName, NavIcon } from "./nav";

const ACTIONS: { href: string; label: string; icon: IconName }[] = [
  { href: "/seller/new", label: "Thêm sản phẩm", icon: "plus" },
  { href: "/seller/orders", label: "Quản lý đơn hàng", icon: "orders" },
  { href: "/seller/wallet", label: "Ví người bán", icon: "wallet" },
  { href: "/seller/ads", label: "Quảng cáo", icon: "megaphone" },
  { href: "/seller/bundles", label: "Combo sản phẩm", icon: "layers" },
];

/** Workplace quick actions: outline links (the one brand CTA is the page header's). */
export function QuickActions() {
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>Thao tác nhanh</CardTitle>
          <CardDescription>Đi tới các khu vực hay dùng.</CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-1">
          {ACTIONS.map((a) => (
            <li key={a.href}>
              <Link href={a.href} variant="outline" size="sm" fullWidth>
                <NavIcon name={a.icon} className="h-4 w-4" />
                {a.label}
              </Link>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
