import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { Skeleton } from "./Skeleton";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { descriptions } = components;

export type DescriptionItem = {
  key: string;
  label: ReactNode;
  children: ReactNode;
};

export type DescriptionsProps = NativeProps<"div"> & {
  items: DescriptionItem[];
  title?: ReactNode;
  extra?: ReactNode;
  column?: 1 | 2 | 3;
  bordered?: boolean;
  loading?: boolean;
  fallback?: ReactNode;
  emptyText?: ReactNode;
  loadingLabel?: string;
};

/** Label and value pairs rendered as a description list, with loading, empty and fallback states. */
export function Descriptions({
  items,
  title,
  extra,
  column = 2,
  bordered = true,
  loading = false,
  fallback,
  emptyText = "Nothing to show yet.",
  loadingLabel,
  ...rest
}: DescriptionsProps) {
  const grid = descriptions.columns[column];
  let body: ReactNode;
  if (fallback) {
    body = fallback;
  } else if (loading) {
    const rows = items.length > 0 ? items : [{ key: "a" }, { key: "b" }];
    body = (
      <div aria-busy="true" className={cx(grid, descriptions.loading)}>
        {rows.map((it) => (
          <Skeleton key={it.key} lines={1} label={loadingLabel} />
        ))}
      </div>
    );
  } else if (items.length === 0) {
    body = <p className={descriptions.empty}>{emptyText}</p>;
  } else {
    body = (
      <div className={bordered ? descriptions.bordered : undefined}>
        <dl className={grid}>
          {items.map((it) => (
            <div
              key={it.key}
              className={cx(
                descriptions.item,
                bordered && descriptions.itemBordered,
              )}
            >
              <dt className={descriptions.term}>{it.label}</dt>
              <dd className={descriptions.detail}>{it.children}</dd>
            </div>
          ))}
        </dl>
      </div>
    );
  }

  return (
    <div {...rest} className={descriptions.root}>
      {(title || extra) && (
        <div className={descriptions.header}>
          {title && <h4 className={descriptions.title}>{title}</h4>}
          {extra && <div className={descriptions.extra}>{extra}</div>}
        </div>
      )}
      {body}
    </div>
  );
}
