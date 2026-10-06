// Link-shaped buttons for server components (the Button component renders a <button>).
const base =
  "inline-flex items-center justify-center rounded-lg font-medium transition duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 active:scale-95";

export const linkButton = {
  primary: `${base} bg-action-primary text-text-inverse hover:bg-action-primary-hover shadow-sm py-1.5 px-3 text-xs`,
  outline: `${base} border border-border-strong bg-surface-card text-text-primary hover:bg-surface-muted shadow-sm py-1.5 px-3 text-xs`,
  outlineMd: `${base} border border-border-strong bg-surface-card text-text-primary hover:bg-surface-muted shadow-sm py-2 px-4 text-sm`,
};
