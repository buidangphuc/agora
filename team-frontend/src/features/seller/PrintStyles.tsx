/**
 * Print stylesheet of the packing slip: hides the site header and footer, the
 * seller navigation chrome (already `print:hidden`) and the toast stack, so only
 * the slip region prints. Plain selectors, no `>` (React escapes it in text).
 */
const PRINT_CSS =
  "@media print { header, footer, [data-print-hidden], .pointer-events-none.fixed { display: none !important; } body { background: white; } }";

export function PrintStyles() {
  return <style>{PRINT_CSS}</style>;
}
