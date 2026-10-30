import type { ReactNode } from "react";

export default function InternalLayout({ children }: { children: ReactNode }) {
  return <section aria-label="Internal area">{children}</section>;
}
