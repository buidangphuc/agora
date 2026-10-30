"use client";

import { components } from "@fe-platform/tokens";
import * as RadixDialog from "@radix-ui/react-dialog";
import { type ReactNode, useRef } from "react";
import { Button } from "./Button";
import { cx } from "./cx";

const { dialog } = components;

export type DialogProps = {
  title: string;
  description?: string;
  triggerLabel?: string;
  closeLabel?: string;
  size?: keyof typeof dialog.size;
  footer?: ReactNode;
  children?: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

/** Modal for a focused task or confirmation: required title, optional description and footer actions; without `triggerLabel` it is controlled by `open` and returns focus to the opener. */
export function Dialog({
  title,
  description,
  triggerLabel,
  closeLabel = "Close",
  size = "md",
  footer,
  children,
  open,
  onOpenChange,
}: DialogProps) {
  const opener = useRef<HTMLElement | null>(null);

  const rememberOpener = () => {
    const active = document.activeElement;
    opener.current = active instanceof HTMLElement ? active : null;
  };

  const restoreFocus = (event: Event) => {
    event.preventDefault();
    if (opener.current?.isConnected) opener.current.focus();
  };

  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      {triggerLabel && (
        <RadixDialog.Trigger asChild>
          <Button variant="secondary">{triggerLabel}</Button>
        </RadixDialog.Trigger>
      )}
      <RadixDialog.Portal>
        <RadixDialog.Overlay className={dialog.overlay} />
        <RadixDialog.Content
          asChild
          aria-modal="true"
          onOpenAutoFocus={triggerLabel ? undefined : rememberOpener}
          onCloseAutoFocus={triggerLabel ? undefined : restoreFocus}
        >
          <dialog open className={cx(dialog.content, dialog.size[size])}>
            <div className={dialog.header}>
              <div>
                <RadixDialog.Title className={dialog.title}>
                  {title}
                </RadixDialog.Title>
                {description && (
                  <RadixDialog.Description className={dialog.description}>
                    {description}
                  </RadixDialog.Description>
                )}
              </div>
              <RadixDialog.Close
                className={dialog.close}
                aria-label={closeLabel}
              >
                <svg
                  className={dialog.closeIcon}
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  strokeWidth={2}
                  aria-hidden="true"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </RadixDialog.Close>
            </div>
            <div className={dialog.body}>{children}</div>
            {footer && <div className={dialog.footer}>{footer}</div>}
          </dialog>
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
