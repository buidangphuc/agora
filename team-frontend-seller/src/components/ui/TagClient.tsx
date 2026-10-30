"use client";

import { ClosableTag, type ClosableTagProps, type Tone } from "@fe-platform/ui";

export interface TagClientProps
  extends Omit<ClosableTagProps, "tone" | "closeLabel"> {
  color?: Tone;
  closeLabel?: string;
}

/** Closable Tag: the only client part of the Tag component. */
export function TagClient({
  color,
  closeLabel = "Xoá",
  ...props
}: TagClientProps) {
  return <ClosableTag {...props} tone={color} closeLabel={closeLabel} />;
}
