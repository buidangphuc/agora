import Link from "next/link";
import React from "react";
import { focusRing } from "./focus";

export interface BreadcrumbItem {
  label: React.ReactNode;
  /** Omit on the last item (the current page). */
  href?: string;
}

export interface BreadcrumbProps {
  items: BreadcrumbItem[];
  className?: string;
}

/**
 * Ant Design `Breadcrumb`. Server-compatible; the last item is the current
 * page (`aria-current="page"`, not a link).
 */
export function Breadcrumb({ items, className = "" }: BreadcrumbProps) {
  return (
    <nav aria-label="Breadcrumb" className={className}>
      <ol className="flex flex-wrap items-center gap-1.5 text-xs text-text-secondary">
        {items.map((item, index) => {
          const isLast = index === items.length - 1;
          return (
            <li
              key={item.href ?? "current"}
              className="flex items-center gap-1.5"
            >
              {item.href && !isLast ? (
                <Link
                  href={item.href}
                  className={`rounded-xs transition duration-150 hover:text-action-primary ${focusRing}`}
                >
                  {item.label}
                </Link>
              ) : (
                <span
                  aria-current={isLast ? "page" : undefined}
                  className={isLast ? "font-medium text-text-primary" : ""}
                >
                  {item.label}
                </span>
              )}
              {!isLast && (
                <span aria-hidden="true" className="text-text-disabled">
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
