import React from "react";
import { TagClient } from "./TagClient";
import { TagView, type TagViewProps } from "./tagView";

export type { TagColor } from "./tagView";

export interface TagProps extends Omit<TagViewProps, "closeButton"> {
  /** Adds a close button (turns the tag into a client island). */
  closable?: boolean;
  onClose?: () => void;
  closeLabel?: string;
}

/**
 * Ant Design `Tag`: a label chip with colour presets from the status tones.
 * Server-compatible unless `closable`.
 */
export function Tag({
  closable = false,
  onClose,
  closeLabel,
  ...view
}: TagProps) {
  if (closable) {
    return <TagClient {...view} onClose={onClose} closeLabel={closeLabel} />;
  }
  return <TagView {...view} />;
}
