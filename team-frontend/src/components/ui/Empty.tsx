import React from "react";

export interface EmptyProps {
  /** Replaces the default illustration. */
  image?: React.ReactNode;
  description?: React.ReactNode;
  /** Optional call to action (a Button or link). */
  action?: React.ReactNode;
  className?: string;
}

function DefaultIllustration() {
  return (
    <svg
      viewBox="0 0 64 48"
      className="h-12 w-16 text-neutral-300"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M8 28l8-16h32l8 16" />
      <path d="M8 28v12a4 4 0 004 4h40a4 4 0 004-4V28H42a4 4 0 01-4 4H26a4 4 0 01-4-4H8z" />
    </svg>
  );
}

/** Ant Design `Empty`: illustration, description and optional action. Server-compatible. */
export function Empty({
  image,
  description = "Không có dữ liệu",
  action,
  className = "",
}: EmptyProps) {
  return (
    <div
      className={`flex flex-col items-center justify-center gap-3 px-6 py-10 text-center ${className}`}
    >
      {image ?? <DefaultIllustration />}
      <p className="text-sm text-text-secondary">{description}</p>
      {action && <div className="mt-1">{action}</div>}
    </div>
  );
}
