import { Tabs } from "@/components/ui/Tabs";
import { SortSelect } from "./SortSelect";
import { type SearchState, type SortKey, buildSearchHref } from "./url";

const TAB_SORTS: { id: SortKey; label: string }[] = [
  { id: "relevance", label: "Liên quan" },
  { id: "newest", label: "Mới nhất" },
];

const PRICE_SORTS: { id: SortKey; label: string }[] = [
  { id: "price_asc", label: "Giá thấp đến cao" },
  { id: "price_desc", label: "Giá cao đến thấp" },
];

/**
 * Sort controls. Desktop: link `Tabs` (Liên quan, Mới nhất) plus a price
 * `Select`; mobile: one `Select` with all four. Only sorts the server honours
 * are offered ("Bán chạy" stays hidden until a backend SortBy exists).
 */
export function SortBar({
  currentSort = "relevance",
  totalResults = 0,
  state,
}: {
  currentSort?: string;
  totalResults?: number;
  /** Current URL state, so sort links keep the other params (additive). */
  state?: SearchState;
}) {
  const base: SearchState = state ?? {
    q: "",
    category: "",
    seller: "",
    sort: "relevance",
    attrs: {},
    page: 1,
  };
  const hrefs = Object.fromEntries(
    [...TAB_SORTS, ...PRICE_SORTS].map((s) => [
      s.id,
      buildSearchHref(base, { sort: s.id }),
    ]),
  );
  const active = (
    [...TAB_SORTS, ...PRICE_SORTS].some((s) => s.id === currentSort)
      ? currentSort
      : "relevance"
  ) as SortKey;
  const priceActive = PRICE_SORTS.some((s) => s.id === active);

  return (
    <div className="flex flex-col gap-3 rounded-xl border border-border-subtle bg-surface-card p-3 shadow-preline-card sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-wrap items-center gap-3">
        <span className="hidden text-sm text-text-secondary sm:inline">
          Sắp xếp theo:
        </span>

        {/* Desktop: tabs + price select */}
        <div className="hidden items-center gap-2 sm:flex">
          <Tabs
            variant="pills"
            items={TAB_SORTS.map((s) => ({ id: s.id, label: s.label }))}
            activeId={priceActive ? undefined : active}
            hrefFor={(id) => hrefs[id] ?? "/search"}
          />
          <SortSelect
            label="Sắp xếp theo giá"
            placeholder="Giá"
            className="w-44"
            value={priceActive ? active : ""}
            hrefs={hrefs}
            options={PRICE_SORTS.map((s) => ({ value: s.id, label: s.label }))}
          />
        </div>

        {/* Mobile: one select with every option */}
        <SortSelect
          label="Sắp xếp"
          className="w-full sm:hidden"
          value={active}
          hrefs={hrefs}
          options={[...TAB_SORTS, ...PRICE_SORTS].map((s) => ({
            value: s.id,
            label: s.label,
          }))}
        />
      </div>

      <p className="text-sm text-text-secondary">
        Tìm thấy{" "}
        <strong className="font-semibold text-action-primary">
          {totalResults}
        </strong>{" "}
        kết quả
      </p>
    </div>
  );
}
