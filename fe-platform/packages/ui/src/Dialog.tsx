"use client";

import { components } from "@fe-platform/tokens";
import * as RadixDialog from "@radix-ui/react-dialog";
import type { ReactNode } from "react";
import { Button } from "./Button";

const { dialog } = components;

export type DialogProps = {
  triggerLabel: string;
  title: string;
  description: string;
  children?: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

/** Modal for a focused task or confirmation. Title and description are required. */
export function Dialog({
  triggerLabel,
  title,
  description,
  children,
  open,
  onOpenChange,
}: DialogProps) {
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      <RadixDialog.Trigger asChild>
        <Button variant="secondary">{triggerLabel}</Button>
      </RadixDialog.Trigger>
      <RadixDialog.Portal>
        <RadixDialog.Overlay className={dialog.overlay} />
        <RadixDialog.Content
          className={`${dialog.content} left-1/2 top-1/2 w-full max-w-lg -translate-x-1/2 -translate-y-1/2`}
        >
          <RadixDialog.Title className={dialog.title}>
            {title}
          </RadixDialog.Title>
          <RadixDialog.Description className={dialog.description}>
            {description}
          </RadixDialog.Description>
          <div className="py-4">{children}</div>
          <RadixDialog.Close asChild>
            <Button variant="ghost">Close</Button>
          </RadixDialog.Close>
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
