import { Link } from "@/components/ui/Link";
import type { ReactNode } from "react";

type Variant = "primary" | "outline" | "ghost";

const sizes = { sm: "xs", md: "sm" } as const;

/** A link styled as a Button. */
export function LinkButton({
  href,
  children,
  variant = "outline",
  size = "md",
}: {
  href: string;
  children: ReactNode;
  variant?: Variant;
  size?: "sm" | "md";
}) {
  return (
    <Link href={href} variant={variant} size={sizes[size]}>
      {children}
    </Link>
  );
}
