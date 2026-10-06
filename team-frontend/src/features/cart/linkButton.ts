/**
 * Class names that make a next/link look like a core Button (a Link must not
 * wrap a <button>). Same tokens and focus ring as Button.
 */
const base =
  "inline-flex items-center justify-center rounded-lg px-5 py-2.5 text-base font-medium transition duration-150 select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 active:scale-95";

export const linkButtonPrimary = `${base} bg-action-primary text-text-inverse shadow-sm hover:bg-action-primary-hover`;

export const linkButtonOutline = `${base} border border-border-strong bg-surface-card text-text-primary shadow-sm hover:bg-surface-muted`;
