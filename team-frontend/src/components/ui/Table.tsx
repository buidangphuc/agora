import { Table as PlatformTable } from "@fe-platform/ui";
import type React from "react";

export interface TableColumn<T> {
  key: string;
  title: string;
  dataIndex?: keyof T & string;
  render?: (row: T, index: number) => React.ReactNode;
  align?: "left" | "center" | "right";
  sticky?: boolean;
  hideBelow?: "sm" | "md";
}

export interface TableProps<T> {
  caption: string;
  columns: TableColumn<T>[];
  dataSource: T[];
  rowKey: (keyof T & string) | ((row: T, index: number) => string);
  loading?: boolean;
  loadingRows?: number;
  error?: string;
  onRetry?: () => void;
  emptyText?: string;
  rowProps?: (row: T, index: number) => { "data-testid": string };
}

export function Table<T>({
  caption,
  columns,
  dataSource,
  rowKey,
  loading,
  loadingRows,
  error,
  onRetry,
  emptyText = "Không có dữ liệu",
  rowProps,
}: TableProps<T>) {
  return (
    <PlatformTable<T>
      caption={caption}
      captionHidden
      columns={columns.map((c) => ({
        key: c.key,
        header: c.title,
        align: c.align,
        sticky: c.sticky,
        hideBelow: c.hideBelow,
        render: (row: T, i: number) =>
          c.render
            ? c.render(row, i)
            : c.dataIndex !== undefined
              ? String(row[c.dataIndex] ?? "")
              : null,
      }))}
      rows={dataSource}
      rowKey={(row, i) =>
        typeof rowKey === "function" ? rowKey(row, i) : String(row[rowKey])
      }
      rowTestId={rowProps && ((row, i) => rowProps(row, i)["data-testid"])}
      emptyText={emptyText}
      loading={loading}
      loadingRows={loadingRows}
      error={error}
      onRetry={onRetry}
      retryLabel="Thử lại"
    />
  );
}
