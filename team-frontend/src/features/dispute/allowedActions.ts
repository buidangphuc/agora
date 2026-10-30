import type { ViewDispute } from "@/lib/gateway/disputes";

export function isOpenDispute(d: ViewDispute): boolean {
  return d.status === "OPEN" || d.status === "INVESTIGATING";
}
