import type { ReactNode } from "react";

import { AuthEventBeacon } from "@/features/auth/AuthEventBeacon";
import { ConsumerShell } from "@/components/shell/ConsumerShell";

/** Consumer chrome for every route except checkout. */
export default function ShopLayout({ children }: { children: ReactNode }) {
  return (
    <ConsumerShell>
      <AuthEventBeacon />
      {children}
    </ConsumerShell>
  );
}
