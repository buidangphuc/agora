"use client";

import { components } from "@fe-platform/tokens";
import * as RadixDialog from "@radix-ui/react-dialog";
import { type ReactNode, useRef } from "react";
import { cx } from "./cx";

const { drawer } = components;

export type DrawerProps = {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  label?: string;
  description?: string;
  children?: ReactNode;
  footer?: ReactNode;
  placement?: "left" | "right";
  size?: "sm" | "md" | "lg";
  closeLabel?: string;
};

/** Side panel for filters, navigation or details, with modal focus trap, Escape and overlay close. */
export function Drawer({
  open,
  onClose,
  title,
  label = "Panel",
  description,
  children,
  footer,
  placement = "right",
  size = "md",
  closeLabel = "Close",
}: DrawerProps) {
  const opener = useRef<HTMLElement | null>(null);
  return (
    <RadixDialog.Root
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <RadixDialog.Portal>
        <RadixDialog.Overlay className={drawer.overlay} />
        <RadixDialog.Content
          aria-modal="true"
          {...(description ? {} : { "aria-describedby": undefined })}
          onOpenAutoFocus={() => {
            opener.current =
              document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null;
          }}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            opener.current?.focus();
          }}
          className={cx(
            drawer.content,
            drawer.placement[placement],
            drawer.size[size],
          )}
        >
          <div className={drawer.header}>
            <div>
              <RadixDialog.Title className={title ? drawer.title : "sr-only"}>
                {title ?? label}
              </RadixDialog.Title>
              {description && (
                <RadixDialog.Description className={drawer.description}>
                  {description}
                </RadixDialog.Description>
              )}
            </div>
            <RadixDialog.Close aria-label={closeLabel} className={drawer.close}>
              <svg
                className={drawer.icon}
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </RadixDialog.Close>
          </div>
          <div className={drawer.body}>{children}</div>
          {footer && <div className={drawer.footer}>{footer}</div>}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
