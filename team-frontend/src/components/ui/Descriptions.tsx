import React from "react";
import { ErrorState } from "./DataState";
import { Empty } from "./Empty";
import { Skeleton } from "./Skeleton";

export interface DescriptionItem {
  key: string;
  label: React.ReactNode;
  children: React.ReactNode;
  span?: number;
}

export interface DescriptionsProps {
  title?: React.ReactNode;
  extra?: React.ReactNode;
  items: DescriptionItem[];
  column?: 1 | 2 | 3;
  bordered?: boolean;
  /** Renders skeleton rows of the same footprint instead of the items. */
  loading?: boolean;
  /** Replaces the body with an inline error Alert. */
  error?: React.ReactNode;
  onRetry?: () => void;
  /** Text of the Empty state when `items` is empty. */
  emptyText?: React.ReactNode;
  className?: string;
}

/**
 * Ant Design-style Descriptions pattern.
 * Displays key-value pairs for technical specifications, order summaries, and KYC details.
 * Rendered as a description list (dl / dt / dd).
 */
export function Descriptions({
  title,
  extra,
  items,
  column = 2,
  bordered = true,
  loading = false,
  error,
  onRetry,
  emptyText,
  className = "",
}: DescriptionsProps) {
  const colClass =
    column === 1
      ? "grid-cols-1"
      : column === 3
        ? "grid-cols-1 sm:grid-cols-2 md:grid-cols-3"
        : "grid-cols-1 sm:grid-cols-2";

  let body: React.ReactNode;
  if (error) {
    body = <ErrorState error={error} onRetry={onRetry} />;
  } else if (loading) {
    body = (
      <div className={`grid ${colClass} gap-3`} aria-busy="true">
        {(items.length > 0 ? items : [{ key: "a" }, { key: "b" }]).map((it) => (
          <Skeleton key={it.key} variant="text" lines={1} />
        ))}
      </div>
    );
  } else if (items.length === 0) {
    body = <Empty description={emptyText} />;
  } else {
    body = (
      <div
        className={`rounded-xl overflow-hidden ${
          bordered
            ? "border border-border-subtle divide-y divide-border-subtle bg-surface-card"
            : ""
        }`}
      >
        <dl className={`grid ${colClass} divide-x divide-border-subtle`}>
          {items.map((it) => (
            <div
              key={it.key}
              className={`p-3.5 text-xs flex flex-col sm:flex-row sm:items-baseline gap-1.5 sm:gap-4 ${
                bordered ? "hover:bg-surface-muted transition duration-150" : ""
              }`}
            >
              <dt className="font-medium text-text-secondary sm:w-1/3 shrink-0">
                {it.label}
              </dt>
              <dd className="font-semibold text-text-primary break-words flex-1">
                {it.children}
              </dd>
            </div>
          ))}
        </dl>
      </div>
    );
  }

  return (
    <div className={`space-y-3 ${className}`}>
      {(title || extra) && (
        <div className="flex items-center justify-between pb-1">
          {title && (
            <h4 className="text-sm font-bold text-text-primary tracking-tight">
              {title}
            </h4>
          )}
          {extra && <div className="text-xs text-text-secondary">{extra}</div>}
        </div>
      )}
      {body}
    </div>
  );
}
