import type { ReactNode } from "react";

import { AdminShell } from "@/components/shell/AdminShell";
import { ForbiddenView } from "@/components/shell/ForbiddenView";
import { ADMIN_SCOPE } from "@/components/shell/nav";
import { redirectGuestToLoginFromHeader } from "@/features/auth/guardPath";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function AdminLayout({
  children,
}: {
  children: ReactNode;
}) {
  const principal = getPrincipal();
  if (!principal) return redirectGuestToLoginFromHeader();
  if (!principal.scopes.includes(ADMIN_SCOPE)) return <ForbiddenView />;
  return <AdminShell userName={principal.name}>{children}</AdminShell>;
}
