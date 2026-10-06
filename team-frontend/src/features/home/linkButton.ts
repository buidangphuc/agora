import { focusRing } from "@/components/ui/focus";

/**
 * A link that looks like a Button (a Button inside an anchor is invalid HTML).
 * Same tokens as Button's md size: 44px tap target, 14px label.
 */
export function linkButtonClass(variant: "primary" | "outline" | "white") {
  const base = `inline-flex min-h-11 items-center justify-center rounded-lg px-4 text-sm font-medium shadow-sm transition duration-150 active:scale-95 ${focusRing}`;
  if (variant === "primary") {
    return `${base} bg-action-primary text-text-inverse hover:bg-action-primary-hover`;
  }
  if (variant === "white") {
    return `${base} bg-surface-card text-text-primary hover:bg-surface-page`;
  }
  return `${base} border border-border-strong bg-surface-card text-text-primary hover:bg-surface-muted`;
}
