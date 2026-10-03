"use client";

import React, { useId } from "react";
import { focusRing } from "./focus";
import { useDialog } from "./useDialog";

export interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  placement?: "left" | "right";
  size?: "sm" | "md" | "lg";
}

// Tier 3: component tokens.
const sizeClasses = { sm: "max-w-xs", md: "max-w-md", lg: "max-w-xl" };
const placementClasses = {
  left: "left-0 border-r",
  right: "right-0 border-l",
};

/**
 * Ant Design `Drawer`: a side panel with the same dialog a11y as Modal
 * (dialog semantics, aria-modal + aria-labelledby, focus trap, Escape,
 * focus return, scroll lock via useDialog).
 */
export function Drawer({
  isOpen,
  onClose,
  title,
  description,
  children,
  footer,
  placement = "right",
  size = "md",
}: DrawerProps) {
  const titleId = useId();
  const descriptionId = useId();
  const ref = useDialog<HTMLDialogElement>({ open: isOpen, onClose });

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 bg-neutral-900/50 backdrop-blur-xs">
      <button
        type="button"
        className="fixed inset-0 cursor-default"
        onClick={onClose}
        tabIndex={-1}
        aria-label="Đóng"
      />
      <dialog
        ref={ref}
        open
        aria-modal="true"
        aria-labelledby={title ? titleId : undefined}
        aria-label={title ? undefined : "Ngăn kéo"}
        aria-describedby={description ? descriptionId : undefined}
        className={`fixed inset-y-0 m-0 flex h-full w-full flex-col p-0 ${sizeClasses[size]} ${placementClasses[placement]} border-border-subtle bg-surface-card text-text-primary shadow-2xl`}
      >
        <div className="flex items-start justify-between border-b border-border-subtle px-5 py-4">
          <div>
            {title && (
              <h3
                id={titleId}
                className="text-base font-semibold leading-tight text-text-primary"
              >
                {title}
              </h3>
            )}
            {description && (
              <p
                id={descriptionId}
                className="mt-1 text-xs text-text-secondary"
              >
                {description}
              </p>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Đóng"
            className={`-mr-2 cursor-pointer rounded-lg p-1.5 text-text-disabled transition duration-150 hover:bg-surface-page hover:text-text-secondary active:scale-95 ${focusRing}`}
          >
            <svg
              className="h-4 w-4"
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
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
        {footer && (
          <div className="flex items-center justify-end gap-2.5 border-t border-border-subtle bg-surface-muted px-5 py-3.5">
            {footer}
          </div>
        )}
      </dialog>
    </div>
  );
}
