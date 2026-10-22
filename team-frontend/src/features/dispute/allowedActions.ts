import type { ViewDispute } from "@/lib/gateway/disputes";

export type DisputeAction = "investigate" | "note" | "resolve" | "reject";

export function allowedDisputeActions(d: ViewDispute): DisputeAction[] {
  switch (d.status) {
    case "OPEN":
      return ["investigate", "reject"];
    case "INVESTIGATING":
      return ["note", "resolve", "reject"];
    default:
      return [];
  }
}

export function needsRuling(d: ViewDispute): boolean {
  return d.returnId !== "";
}

export function isOpenDispute(d: ViewDispute): boolean {
  return d.status === "OPEN" || d.status === "INVESTIGATING";
}
