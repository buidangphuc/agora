"use client";

import React, { useState } from "react";
import { TagView, type TagViewProps } from "./tagView";

export interface TagClientProps extends Omit<TagViewProps, "closeButton"> {
  onClose?: () => void;
  closeLabel?: string;
}

/** Closable Tag: the only client part of the Tag component. */
export function TagClient({
  onClose,
  closeLabel = "Xoá",
  children,
  ...view
}: TagClientProps) {
  const [open, setOpen] = useState(true);
  if (!open) return null;
  return (
    <TagView
      {...view}
      closeButton={
        <button
          type="button"
          aria-label={closeLabel}
          onClick={() => {
            setOpen(false);
            onClose?.();
          }}
          className="-mr-1 cursor-pointer rounded-xs px-0.5 transition duration-150 hover:opacity-70 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
        >
          <span aria-hidden="true">✕</span>
        </button>
      }
    >
      {children}
    </TagView>
  );
}
