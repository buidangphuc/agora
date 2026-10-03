"use client";

import React, { useEffect } from "react";

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  size?: "sm" | "md" | "lg" | "xl";
}

const sizeClasses = {
  sm: "max-w-md",
  md: "max-w-lg",
  lg: "max-w-2xl",
  xl: "max-w-4xl",
};

export function Modal({
  isOpen,
  onClose,
  title,
  description,
  children,
  footer,
  size = "md",
}: ModalProps) {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    if (isOpen) {
      document.body.style.overflow = "hidden";
      window.addEventListener("keydown", handleKeyDown);
    } else {
      document.body.style.overflow = "unset";
    }
    return () => {
      document.body.style.overflow = "unset";
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-gray-900/50 backdrop-blur-xs flex items-center justify-center p-4">
      <button
        type="button"
        className="fixed inset-0 cursor-default"
        onClick={onClose}
        tabIndex={-1}
        aria-label="Đóng"
      />

      <dialog
        open
        aria-modal="true"
        className={`relative m-0 block p-0 w-full ${sizeClasses[size]} bg-white rounded-xl shadow-2xl border border-gray-100 overflow-hidden transform transition-all z-10`}
      >
        {/* Header */}
        {(title || description) && (
          <div className="px-6 py-4 border-b border-gray-100 flex items-start justify-between">
            <div>
              {title && (
                <h3 className="text-base font-semibold text-gray-900 leading-tight">
                  {title}
                </h3>
              )}
              {description && (
                <p className="text-xs text-gray-500 mt-1">{description}</p>
              )}
            </div>
            <button
              type="button"
              onClick={onClose}
              className="text-gray-400 hover:text-gray-600 rounded-lg p-1.5 hover:bg-gray-100 transition cursor-pointer -mr-2"
              aria-label="Đóng"
            >
              <svg
                className="w-4 h-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <title>Đóng</title>
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
          <div className="px-6 py-3.5 bg-gray-50 border-t border-gray-100 flex items-center justify-end gap-2.5">
            {footer}
          </div>
        )}
      </dialog>
    </div>
  );
}
