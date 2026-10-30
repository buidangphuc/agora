import {
  Pagination as PlatformPagination,
  type PaginationProps as PlatformPaginationProps,
} from "@fe-platform/ui";

export type PaginationProps = Omit<
  PlatformPaginationProps,
  "label" | "previousLabel" | "nextLabel" | "pageLabel"
>;

const pageLabel = (page: number) => `Trang ${page}`;

export function Pagination(props: PaginationProps) {
  return (
    <PlatformPagination
      {...props}
      label="Phân trang"
      previousLabel="Trang trước"
      nextLabel="Trang sau"
      pageLabel={pageLabel}
    />
  );
}
