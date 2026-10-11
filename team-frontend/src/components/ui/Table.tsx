import React from "react";
import { ErrorState } from "./DataState";
import { Empty } from "./Empty";
import { Skeleton } from "./Skeleton";

export interface TableColumn<T> {
  key: string;
  title: React.ReactNode;
  /** Field shown when there is no `render`. */
  dataIndex?: keyof T & string;
  render?: (row: T, index: number) => React.ReactNode;
  align?: "left" | "center" | "right";
  /** Tailwind width / hiding classes for the column (e.g. "w-24", "hidden md:table-cell"). */
  className?: string;
}

export interface TableProps<T> {
  columns: TableColumn<T>[];
  dataSource: T[];
  /** Field name or function giving a stable unique key per row. */
  rowKey: (keyof T & string) | ((row: T, index: number) => string);
  /** Skeleton rows instead of data; the table is marked aria-busy. */
  loading?: boolean;
  /** Skeleton row count while loading. */
  loadingRows?: number;
  /** Inline error Alert in the body; shows a retry button with `onRetry`. */
  error?: React.ReactNode;
  onRetry?: () => void;
  /** Text of the Empty block for an empty dataSource. */
  emptyText?: React.ReactNode;
  /** Extra attributes for a data row (e.g. a `data-testid` hook). */
  rowProps?: (
    row: T,
    index: number,
  ) => React.HTMLAttributes<HTMLTableRowElement> & {
    "data-testid"?: string;
  };
  /** Visually hidden table caption (accessible name). */
  caption?: string;
  className?: string;
}

const alignClass = {
  left: "text-left",
  center: "text-center",
  right: "text-right",
};

/**
 * Ant Design `Table` (columns, row key, empty / loading / error). Always
 * renders the column headers so the footprint is stable. Server-compatible.
 */
export function Table<T>({
  columns,
  dataSource,
  rowKey,
  loading = false,
  loadingRows = 5,
  error,
  onRetry,
  emptyText,
  rowProps,
  caption,
  className = "",
}: TableProps<T>) {
  const span = columns.length;
  const keyOf = (row: T, i: number) =>
    typeof rowKey === "function" ? rowKey(row, i) : String(row[rowKey]);

  let body: React.ReactNode;
  if (error) {
    body = (
      <tr>
        <td colSpan={span} className="p-3">
          <ErrorState error={error} onRetry={onRetry} />
        </td>
      </tr>
    );
  } else if (loading) {
    body = Array.from({ length: loadingRows }, (_, i) => i).map((row) => (
      <tr key={row} className="border-t border-border-subtle">
        {columns.map((c) => (
          <td key={c.key} className={`px-4 py-3 ${c.className ?? ""}`}>
            <Skeleton variant="text" lines={1} />
          </td>
        ))}
      </tr>
    ));
  } else if (dataSource.length === 0) {
    body = (
      <tr>
        <td colSpan={span}>
          <Empty description={emptyText} />
        </td>
      </tr>
    );
  } else {
    body = dataSource.map((row, i) => (
      <tr
        key={keyOf(row, i)}
        {...rowProps?.(row, i)}
        className="border-t border-border-subtle transition duration-150 hover:bg-surface-muted"
      >
        {columns.map((c) => (
          <td
            key={c.key}
            className={`px-4 py-3 text-sm text-text-primary ${alignClass[c.align ?? "left"]} ${c.className ?? ""}`}
          >
            {c.render
              ? c.render(row, i)
              : c.dataIndex !== undefined
                ? String(row[c.dataIndex] ?? "")
                : null}
          </td>
        ))}
      </tr>
    ));
  }

  return (
    <div
      className={`overflow-x-auto rounded-xl border border-border-subtle bg-surface-card ${className}`}
    >
      <table
        className="w-full border-collapse"
        aria-busy={loading ? "true" : undefined}
      >
        {caption && <caption className="sr-only">{caption}</caption>}
        <thead className="bg-surface-muted">
          <tr>
            {columns.map((c) => (
              <th
                key={c.key}
                scope="col"
                className={`px-4 py-3 text-xs font-semibold text-text-secondary ${alignClass[c.align ?? "left"]} ${c.className ?? ""}`}
              >
                {c.title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{body}</tbody>
      </table>
    </div>
  );
}
