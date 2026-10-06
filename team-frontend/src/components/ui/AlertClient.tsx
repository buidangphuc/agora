"use client";

import React, { useState } from "react";
import { AlertView, type AlertViewProps } from "./alertView";

export interface AlertClientProps extends Omit<AlertViewProps, "closeButton"> {
  onClose?: () => void;
}

/** Closable Alert: the only client part of the Alert component. */
export function AlertClient({ onClose, ...view }: AlertClientProps) {
  const [open, setOpen] = useState(true);
  if (!open) return null;
  return (
    <AlertView
      {...view}
      closeButton={
        <button
          type="button"
          aria-label="Đóng"
          onClick={() => {
            setOpen(false);
            onClose?.();
          }}
          className="shrink-0 cursor-pointer rounded-lg p-1 text-text-secondary transition duration-150 hover:text-text-primary active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
        >
          <span aria-hidden="true">✕</span>
        </button>
      }
    />
  );
}
