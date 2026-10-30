"use client";

import { components } from "@fe-platform/tokens";
import { useState } from "react";
import type { Tone } from "./Tag";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { tag, tone: tones } = components;

export type ClosableTagProps = NativeProps<"span"> & {
  tone?: Tone;
  closeLabel: string;
  onClose?: () => void;
};

/** Tag with a labelled close button that hides it and reports the dismissal, such as an active filter. */
export function ClosableTag({
  tone = "neutral",
  closeLabel,
  onClose,
  children,
  ...rest
}: ClosableTagProps) {
  const [open, setOpen] = useState(true);
  if (!open) return null;
  return (
    <span {...rest} className={cx(tag.base, tones[tone])}>
      {children}
      <button
        type="button"
        aria-label={closeLabel}
        className={tag.close}
        onClick={() => {
          setOpen(false);
          onClose?.();
        }}
      >
        <span aria-hidden="true">✕</span>
      </button>
    </span>
  );
}
