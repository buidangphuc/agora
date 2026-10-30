import type { ReactNode } from "react";

import { SellerAuthShell } from "@/components/shell/SellerAuthShell";

export default function AuthLayout({ children }: { children: ReactNode }) {
  return <SellerAuthShell>{children}</SellerAuthShell>;
}
