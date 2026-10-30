const LINKS = [
  { href: "#specs", label: "Chi tiết" },
  { href: "#reviews", label: "Đánh giá" },
  { href: "#qa", label: "Hỏi đáp" },
];

/**
 * Anchor nav of the detail body: plain links to the always-visible sections
 * (`#specs`, `#reviews`, `#qa`). Sticky from `lg`; below it, it scrolls
 * horizontally without wrapping and without sticking. No tabs, no URL state.
 */
export function PdpAnchorNav() {
  return (
    <nav
      aria-label="Nội dung sản phẩm"
      className="z-10 overflow-x-auto rounded-2xl border border-border-subtle bg-surface-card p-1.5 shadow-card lg:sticky lg:top-32"
    >
      <ul className="flex gap-1 whitespace-nowrap">
        {LINKS.map((l) => (
          <li key={l.href}>
            <a
              href={l.href}
              className="flex min-h-touch items-center rounded-xl px-4 text-sm font-medium text-text-secondary transition duration-150 hover:bg-surface-muted hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
            >
              {l.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
