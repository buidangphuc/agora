import React from "react";

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
  className?: string;
}

/**
 * Ant Design-style Descriptions pattern.
 * Displays key-value pairs for technical specifications, order summaries, and KYC details.
 */
export function Descriptions({
  title,
  extra,
  items,
  column = 2,
  bordered = true,
  className = "",
}: DescriptionsProps) {
  const colClass =
    column === 1
      ? "grid-cols-1"
      : column === 3
        ? "grid-cols-1 sm:grid-cols-2 md:grid-cols-3"
        : "grid-cols-1 sm:grid-cols-2";

  return (
    <div className={`space-y-3 ${className}`}>
      {(title || extra) && (
        <div className="flex items-center justify-between pb-1">
          {title && (
            <h4 className="text-sm font-bold text-gray-900 tracking-tight">
              {title}
            </h4>
          )}
          {extra && <div className="text-xs text-gray-500">{extra}</div>}
        </div>
      )}

      <div
        className={`rounded-xl overflow-hidden ${
          bordered
            ? "border border-gray-200/90 divide-y divide-gray-100 bg-white"
            : ""
        }`}
      >
        <div className={`grid ${colClass} divide-x divide-gray-100`}>
          {items.map((it) => (
            <div
              key={it.key}
              className={`p-3.5 text-xs flex flex-col sm:flex-row sm:items-baseline gap-1.5 sm:gap-4 ${
                bordered ? "hover:bg-gray-50/50 transition" : ""
              }`}
            >
              <span className="font-medium text-gray-500 sm:w-1/3 shrink-0">
                {it.label}
              </span>
              <span className="font-semibold text-gray-800 break-words flex-1">
                {it.children}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
