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
      className="z-10 overflow-x-auto rounded-xl border border-border-subtle bg-surface-card shadow-preline-card lg:sticky lg:top-32"
    >
      <ul className="flex gap-6 whitespace-nowrap px-5">
        {LINKS.map((l) => (
          <li key={l.href}>
            <a
              href={l.href}
              className="block rounded-xs py-3 text-sm font-medium text-text-secondary transition duration-150 hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2"
            >
              {l.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
