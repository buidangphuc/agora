"use client";

import { useEffect, useRef } from "react";

/**
 * Internal dialog behaviour shared by Modal and Drawer (ui-core-components
 * design decision 5): focus trap, Escape to close, return focus to the opener
 * and body scroll lock. Hand-written on purpose (no dependency); it does not
 * handle iframes or shadow DOM, which this app does not use.
 */

const FOCUSABLE = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

// Open dialogs, topmost last: only the topmost one reacts to Escape and Tab.
const stack: symbol[] = [];
let lockCount = 0;
let previousOverflow = "";

function lockScroll() {
  if (lockCount === 0) {
    previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
  lockCount += 1;
}

function unlockScroll() {
  lockCount = Math.max(0, lockCount - 1);
  if (lockCount === 0) document.body.style.overflow = previousOverflow;
}

export interface UseDialogOptions {
  open: boolean;
  onClose: () => void;
}

/** Returns a ref to attach to the dialog container element. */
export function useDialog<T extends HTMLElement = HTMLElement>({
  open,
  onClose,
}: UseDialogOptions) {
  const containerRef = useRef<T>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    if (!open) return;
    const container = containerRef.current;
    if (!container) return;

    const id = Symbol("dialog");
    stack.push(id);
    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    lockScroll();

    const focusables = () =>
      Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE));

    if (!container.contains(document.activeElement)) {
      const first = focusables()[0];
      if (first) first.focus();
      else {
        container.tabIndex = -1;
        container.focus();
      }
    }

    const onKeyDown = (e: KeyboardEvent) => {
      if (stack[stack.length - 1] !== id) return;
      if (e.key === "Escape") {
        e.stopPropagation();
        onCloseRef.current();
        return;
      }
      if (e.key !== "Tab") return;
      const items = focusables();
      if (items.length === 0) {
        e.preventDefault();
        container.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      const outside = !container.contains(active);
      if (e.shiftKey && (active === first || outside)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (active === last || outside)) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      const at = stack.indexOf(id);
      if (at >= 0) stack.splice(at, 1);
      unlockScroll();
      if (opener?.isConnected) opener.focus();
    };
  }, [open]);

  return containerRef;
}
