import type { ReactNode } from "react";

import { ConsumerShell } from "@/components/shell/ConsumerShell";

/** Consumer chrome for every route except checkout. */
export default function ShopLayout({ children }: { children: ReactNode }) {
  return <ConsumerShell>{children}</ConsumerShell>;
}
