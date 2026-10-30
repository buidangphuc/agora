import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { Button } from "./Button";
import { cx } from "./cx";

const { table } = components;

export type TableColumn<T = Record<string, ReactNode>> = {
  key: string;
  header: string;
  render?: (row: T, index: number) => ReactNode;
  align?: keyof typeof table.align;
  sticky?: boolean;
  hideBelow?: keyof typeof table.hideBelow;
};

export type TableProps<T = Record<string, ReactNode>> = {
  caption: string;
  captionHidden?: boolean;
  columns: TableColumn<T>[];
  rows: T[];
  rowKey?: (row: T, index: number) => string;
  rowTestId?: (row: T, index: number) => string;
  emptyText?: string;
  loading?: boolean;
  loadingRows?: number;
  error?: string;
  onRetry?: () => void;
  retryLabel?: string;
};

function cellClass<T>(base: string, c: TableColumn<T>) {
  return cx(
    base,
    table.align[c.align ?? "left"],
    c.sticky && table.sticky,
    c.hideBelow && table.hideBelow[c.hideBelow],
  );
}

function defaultKey(row: unknown, index: number) {
  const id = (row as { id?: unknown } | null)?.id;
  return id === undefined || id === null ? String(index) : String(id);
}

/** Read-only data table with a required caption, column-scoped headers, row keys, custom cells and empty, loading and error states. */
export function Table<T = Record<string, ReactNode>>({
  caption,
  captionHidden,
  columns,
  rows,
  rowKey = defaultKey,
  rowTestId,
  emptyText = "Nothing to show yet.",
  loading = false,
  loadingRows = 5,
  error,
  onRetry,
  retryLabel = "Try again",
}: TableProps<T>) {
  let body: ReactNode;
  if (error) {
    body = (
      <tr>
        <td colSpan={columns.length} className={table.td}>
          <div role="alert" className={table.error}>
            <p>{error}</p>
            {onRetry && (
              <Button variant="secondary" size="sm" onClick={onRetry}>
                {retryLabel}
              </Button>
            )}
          </div>
        </td>
      </tr>
    );
  } else if (loading) {
    body = Array.from({ length: loadingRows }, (_, i) => i).map((slot) => (
      <tr key={slot}>
        {columns.map((c) => (
          <td key={c.key} className={cellClass(table.td, c)}>
            <span aria-hidden="true" className={table.skeleton} />
          </td>
        ))}
      </tr>
    ));
  } else if (rows.length === 0) {
    body = (
      <tr>
        <td colSpan={columns.length} className={table.empty}>
          {emptyText}
        </td>
      </tr>
    );
  } else {
    body = rows.map((row, i) => (
      <tr
        key={rowKey(row, i)}
        data-testid={rowTestId?.(row, i)}
        className={table.row}
      >
        {columns.map((c) => (
          <td key={c.key} className={cellClass(table.td, c)}>
            {c.render
              ? c.render(row, i)
              : (row as Record<string, ReactNode>)[c.key]}
          </td>
        ))}
      </tr>
    ));
  }

  return (
    <div className={table.wrap}>
      <table className={table.root} aria-busy={loading ? true : undefined}>
        <caption className={captionHidden ? "sr-only" : table.caption}>
          {caption}
        </caption>
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.key} scope="col" className={cellClass(table.th, c)}>
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{body}</tbody>
      </table>
    </div>
  );
}
