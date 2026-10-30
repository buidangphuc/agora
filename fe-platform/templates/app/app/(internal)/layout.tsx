import { ToastProvider } from "@fe-platform/ui";
import type { ReactNode } from "react";
import { requireSession } from "../../lib/session";

export default async function InternalLayout({
  children,
}: {
  children: ReactNode;
}) {
  await requireSession();
  return (
    <ToastProvider>
      <section aria-label="Internal area">{children}</section>
    </ToastProvider>
  );
}
