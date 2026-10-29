import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";

const { table } = components;

export type TableColumn = { key: string; header: string };

export type TableProps = {
  caption: string;
  columns: TableColumn[];
  rows: Record<string, ReactNode>[];
  emptyText?: string;
};

/** Read-only data table with a required caption and column-scoped headers. */
export function Table({
  caption,
  columns,
  rows,
  emptyText = "Nothing to show yet.",
}: TableProps) {
  return (
    <table className={table.root}>
      <caption className={table.caption}>{caption}</caption>
      <thead>
        <tr>
          {columns.map((c) => (
            <th key={c.key} scope="col" className={table.th}>
              {c.header}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.length === 0 ? (
          <tr>
            <td colSpan={columns.length} className={table.empty}>
              {emptyText}
            </td>
          </tr>
        ) : (
          rows.map((row, i) => (
            <tr key={String(row.id ?? i)}>
              {columns.map((c) => (
                <td key={c.key} className={table.td}>
                  {row[c.key]}
                </td>
              ))}
            </tr>
          ))
        )}
      </tbody>
    </table>
  );
}
