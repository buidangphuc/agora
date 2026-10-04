export type OrderDetailTab = "timeline" | "returns";

/** `?tab=` value of the order detail; anything but "returns" is the timeline. */
export function parseDetailTab(value: string | undefined): OrderDetailTab {
  return value === "returns" ? "returns" : "timeline";
}
