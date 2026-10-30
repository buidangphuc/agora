import {
  Tag as PlatformTag,
  type TagProps as PlatformTagProps,
  type Tone,
} from "@fe-platform/ui";
import { TagClient } from "./TagClient";

export type TagColor = Tone;

export interface TagProps extends Omit<PlatformTagProps, "tone"> {
  color?: TagColor;
  closable?: boolean;
  onClose?: () => void;
  closeLabel?: string;
}

/**
 * Ant Design `Tag`: a label chip with colour presets from the status tones.
 * Server-compatible unless `closable`.
 */
export function Tag({
  color,
  closable = false,
  onClose,
  closeLabel,
  ...props
}: TagProps) {
  if (closable) {
    return (
      <TagClient
        {...props}
        color={color}
        onClose={onClose}
        closeLabel={closeLabel}
      />
    );
  }
  return <PlatformTag {...props} tone={color} />;
}
