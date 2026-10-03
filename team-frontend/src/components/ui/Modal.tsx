"use client";

import React, { useId } from "react";
import { useDialog } from "./useDialog";

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  size?: "sm" | "md" | "lg" | "xl";
}

// Tier 3: component tokens.
const sizeClasses = {
  sm: "max-w-md",
  md: "max-w-lg",
  lg: "max-w-2xl",
  xl: "max-w-4xl",
};

const closeFocus =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring";

/**
 * Ant Design `Modal`. Native `<dialog open>` carries the implicit dialog role;
 * `aria-modal` + `aria-labelledby` the title. Focus is trapped, Escape closes
 * and focus returns to the opener (useDialog).
 */
export function Modal({
  isOpen,
  onClose,
  title,
  description,
  children,
  footer,
  size = "md",
}: ModalProps) {
  const titleId = useId();
  const descriptionId = useId();
  const dialogRef = useDialog<HTMLDialogElement>({ open: isOpen, onClose });

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-neutral-900/50 backdrop-blur-xs flex items-center justify-center p-4">
      <button
        type="button"
        className="fixed inset-0 cursor-default"
        onClick={onClose}
        tabIndex={-1}
        aria-label="Đóng"
      />

      <dialog
        ref={dialogRef}
        open
        aria-modal="true"
        aria-labelledby={title ? titleId : undefined}
        aria-label={title ? undefined : "Hộp thoại"}
        aria-describedby={description ? descriptionId : undefined}
        className={`relative m-0 block p-0 w-full ${sizeClasses[size]} bg-surface-card text-text-primary rounded-2xl shadow-2xl border border-border-subtle overflow-hidden z-10`}
      >
        {/* Header */}
        {(title || description) && (
          <div className="px-6 py-4 border-b border-border-subtle flex items-start justify-between">
            <div>
              {title && (
                <h3
                  id={titleId}
                  className="text-base font-semibold text-text-primary leading-tight"
                >
                  {title}
                </h3>
              )}
              {description && (
                <p
                  id={descriptionId}
                  className="text-xs text-text-secondary mt-1"
                >
                  {description}
                </p>
              )}
            </div>
            <button
              type="button"
              onClick={onClose}
              className={`text-text-disabled hover:text-text-secondary rounded-lg p-1.5 hover:bg-surface-page transition duration-150 cursor-pointer -mr-2 ${closeFocus}`}
              aria-label="Đóng"
            >
              <svg
                className="w-4 h-4"
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
        )}

        {/* Content Body */}
        <div className="px-6 py-5">{children}</div>

        {/* Footer */}
        {footer && (
          <div className="px-6 py-3.5 bg-surface-muted border-t border-border-subtle flex items-center justify-end gap-2.5">
            {footer}
          </div>
        )}
      </dialog>
    </div>
  );
}
