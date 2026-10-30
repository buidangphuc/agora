import { components } from "@fe-platform/tokens";
import NextLink from "next/link";
import { cx } from "./cx";

const { pagination } = components;

export type PaginationProps = {
  current: number;
  total: number;
  pageSize?: number;
  hrefFor: (page: number) => string;
  label?: string;
  previousLabel?: string;
  nextLabel?: string;
  pageLabel?: (page: number) => string;
};

type Slot = number | "gap-start" | "gap-end";

function slots(current: number, pages: number): Slot[] {
  if (pages <= 7) return Array.from({ length: pages }, (_, i) => i + 1);
  const out: Slot[] = [1];
  const from = Math.max(2, current - 1);
  const to = Math.min(pages - 1, current + 1);
  if (from > 2) out.push("gap-start");
  for (let p = from; p <= to; p++) out.push(p);
  if (to < pages - 1) out.push("gap-end");
  out.push(pages);
  return out;
}

const idle = cx(pagination.cell, pagination.idle);
const disabled = cx(pagination.cell, pagination.disabled);

/** Page links driven by the URL: `hrefFor(page)` builds each link, the current page carries `aria-current="page"`, long ranges collapse around it, and a single page renders nothing. */
export function Pagination({
  current,
  total,
  pageSize = 10,
  hrefFor,
  label = "Pagination",
  previousLabel = "Previous page",
  nextLabel = "Next page",
  pageLabel = (page) => `Page ${page}`,
}: PaginationProps) {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  if (pages <= 1) return null;
  const page = Math.min(Math.max(1, current), pages);

  return (
    <nav aria-label={label}>
      <ul className={pagination.list}>
        <li>
          {page > 1 ? (
            <NextLink
              href={hrefFor(page - 1)}
              rel="prev"
              aria-label={previousLabel}
              className={idle}
            >
              ‹
            </NextLink>
          ) : (
            <span
              aria-disabled="true"
              aria-label={previousLabel}
              className={disabled}
            >
              ‹
            </span>
          )}
        </li>
        {slots(page, pages).map((slot) => (
          <li key={slot}>
            {typeof slot !== "number" ? (
              <span
                aria-hidden="true"
                className={cx(pagination.cell, pagination.gap)}
              >
                …
              </span>
            ) : (
              <NextLink
                href={hrefFor(slot)}
                aria-label={pageLabel(slot)}
                aria-current={slot === page ? "page" : undefined}
                className={
                  slot === page ? cx(pagination.cell, pagination.active) : idle
                }
              >
                {slot}
              </NextLink>
            )}
          </li>
        ))}
        <li>
          {page < pages ? (
            <NextLink
              href={hrefFor(page + 1)}
              rel="next"
              aria-label={nextLabel}
              className={idle}
            >
              ›
            </NextLink>
          ) : (
            <span
              aria-disabled="true"
              aria-label={nextLabel}
              className={disabled}
            >
              ›
            </span>
          )}
        </li>
      </ul>
    </nav>
  );
}
