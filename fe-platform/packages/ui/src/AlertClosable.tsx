"use client";

import { useState } from "react";
import { Button } from "./Button";
import { AlertBody, type AlertBodyProps } from "./alertBody";

export function AlertClosable({
  onClose,
  closeLabel,
  ...body
}: AlertBodyProps & { onClose?: () => void; closeLabel: string }) {
  const [open, setOpen] = useState(true);
  if (!open) return null;
  return (
    <AlertBody
      {...body}
      close={
        <Button
          variant="ghost"
          size="sm"
          aria-label={closeLabel}
          onClick={() => {
            setOpen(false);
            onClose?.();
          }}
        >
          <span aria-hidden="true">✕</span>
        </Button>
      }
    />
  );
}
