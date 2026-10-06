import Link from "next/link";
import type { ReactNode } from "react";

import { focusRing } from "@/components/ui/focus";

/** A primary-looking link for Empty / Result actions that navigate (Button is a `<button>`). */
export function LinkButton({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className={`inline-flex min-h-10 items-center justify-center rounded-lg bg-action-primary px-4 text-sm font-medium text-text-inverse shadow-sm transition duration-150 hover:bg-action-primary-hover ${focusRing}`}
    >
      {children}
    </Link>
  );
}
