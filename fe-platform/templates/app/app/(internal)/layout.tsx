import { ToastProvider } from "@fe-platform/ui";
import type { ReactNode } from "react";

export default function InternalLayout({ children }: { children: ReactNode }) {
  return (
    <ToastProvider>
      <section aria-label="Internal area">{children}</section>
    </ToastProvider>
  );
}
