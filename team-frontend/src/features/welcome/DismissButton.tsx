"use client";

import { useTransition } from "react";

import { dismissWelcome } from "./actions";

export function DismissButton({
  part,
  label,
  className,
}: {
  part: "banner" | "checklist";
  label: string;
  className?: string;
}) {
  const [pending, start] = useTransition();
  return (
    <button
      type="button"
      disabled={pending}
      className={className}
      onClick={() => start(() => dismissWelcome(part))}
    >
      {label}
    </button>
  );
}
