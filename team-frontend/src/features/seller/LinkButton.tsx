import Link from "next/link";
import type { ReactNode } from "react";

type Variant = "primary" | "outline" | "ghost";

// Tier 3 for anchor-buttons: mirrors Button's variant map (Button renders a
// <button>, and a link must stay an <a> so navigation works without JS).
const variants: Record<Variant, string> = {
  primary:
    "bg-action-primary text-text-inverse hover:bg-action-primary-hover shadow-sm",
  outline:
    "border border-border-strong bg-surface-card text-text-primary hover:bg-surface-muted shadow-sm",
  ghost: "text-text-primary hover:bg-surface-page",
};

const sizes = {
  sm: "px-3 py-1.5 text-xs",
  md: "px-4 py-2 text-sm",
};

/** A link styled as a Button. */
export function LinkButton({
  href,
  children,
  variant = "outline",
  size = "md",
  className = "",
}: {
  href: string;
  children: ReactNode;
  variant?: Variant;
  size?: "sm" | "md";
  className?: string;
}) {
  return (
    <Link
      href={href}
      className={`inline-flex items-center justify-center rounded-lg font-medium transition duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 active:scale-95 ${variants[variant]} ${sizes[size]} ${className}`}
    >
      {children}
    </Link>
  );
}
