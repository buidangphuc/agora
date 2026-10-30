import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";

const { empty } = components;

export type EmptyProps = {
  description: ReactNode;
  image?: ReactNode;
  action?: ReactNode;
  "data-testid"?: string;
};

/** Placeholder for a list or page with no data: illustration, description and an optional action. */
export function Empty({
  description,
  image,
  action,
  "data-testid": testId,
}: EmptyProps) {
  return (
    <div className={empty.root} data-testid={testId}>
      {image ?? (
        <svg
          viewBox="0 0 64 48"
          className={empty.image}
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
      )}
      <p className={empty.description}>{description}</p>
      {action && <div className={empty.action}>{action}</div>}
    </div>
  );
}
