import { Link } from "@/components/ui/Link";
import type { ReactNode } from "react";

/** A primary-looking link for Empty / Result actions that navigate (Button is a `<button>`). */
export function LinkButton({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link href={href} variant="primary" size="sm">
      {children}
    </Link>
  );
}
