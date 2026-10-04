import type { ReactNode } from "react";

import { Breadcrumb, type BreadcrumbItem } from "@/components/ui/Breadcrumb";

/**
 * Page header of every seller page (Ant Pro PageContainer): Breadcrumb
 * (Kênh người bán > page), 20px title, description and one primary action.
 */
export function SellerPageHeader({
  title,
  description,
  trail = [],
  action,
}: {
  title: string;
  description?: string;
  /** Intermediate crumbs between the seller root and the current page. */
  trail?: BreadcrumbItem[];
  /** The page's single primary CTA (right-aligned). */
  action?: ReactNode;
}) {
  const items: BreadcrumbItem[] = [
    { label: "Kênh người bán", href: "/seller" },
    ...trail,
    { label: title },
  ];
  return (
    <header className="space-y-3 print:hidden">
      <Breadcrumb items={items} />
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-xl font-bold tracking-tight text-text-primary">
            {title}
          </h1>
          {description && (
            <p className="mt-1 text-sm text-text-secondary">{description}</p>
          )}
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
    </header>
  );
}
