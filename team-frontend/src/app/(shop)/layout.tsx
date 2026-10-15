import type { ReactNode } from "react";

import { ConsumerShell } from "@/components/shell/ConsumerShell";
import { AuthEventBeacon } from "@/features/auth/AuthEventBeacon";

/** Consumer chrome for every route except checkout. */
export default function ShopLayout({ children }: { children: ReactNode }) {
  return (
    <ConsumerShell>
      <AuthEventBeacon />
      {children}
    </ConsumerShell>
  );
}
