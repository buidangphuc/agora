import { components } from "@fe-platform/tokens";
import NextLink from "next/link";
import type { ReactNode } from "react";

const { breadcrumb } = components;

export type BreadcrumbItem = {
  label: ReactNode;
  href?: string;
};

export type BreadcrumbProps = {
  items: BreadcrumbItem[];
  label?: string;
};

/** Trail of ancestor pages; ancestors with an `href` are links and the last item is the current page (`aria-current="page"`, never a link). */
export function Breadcrumb({ items, label = "Breadcrumb" }: BreadcrumbProps) {
  return (
    <nav aria-label={label}>
      <ol className={breadcrumb.list}>
        {items.map((item, index) => {
          const isLast = index === items.length - 1;
          return (
            <li key={item.href ?? "current"} className={breadcrumb.item}>
              {item.href && !isLast ? (
                <NextLink href={item.href} className={breadcrumb.link}>
                  {item.label}
                </NextLink>
              ) : (
                <span
                  aria-current={isLast ? "page" : undefined}
                  className={isLast ? breadcrumb.current : undefined}
                >
                  {item.label}
                </span>
              )}
              {!isLast && (
                <span aria-hidden="true" className={breadcrumb.separator}>
                  /
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
