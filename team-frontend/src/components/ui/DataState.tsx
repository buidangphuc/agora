import React from "react";
import { Alert } from "./Alert";
import { Button } from "./Button";

export interface ErrorStateProps {
  /** Error message; the state renders only when set. */
  error: React.ReactNode;
  /** Retry handler. Without it no retry button is shown. */
  onRetry?: () => void;
  className?: string;
}

/**
 * Shared error state of the data components (Table, Timeline, Descriptions,
 * Statistic): an inline error Alert with a retry action.
 */
export function ErrorState({ error, onRetry, className }: ErrorStateProps) {
  return (
    <Alert
      type="error"
      description={error}
      className={className}
      action={
        onRetry ? (
          <Button size="sm" variant="outline" onClick={onRetry}>
            Thử lại
          </Button>
        ) : undefined
      }
    />
  );
}
