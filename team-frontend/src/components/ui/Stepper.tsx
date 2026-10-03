import React from "react";

export interface StepItem {
  id: string | number;
  title: string;
  description?: string;
  status: "complete" | "current" | "upcoming" | "failed";
}

export interface StepperProps {
  steps: StepItem[];
  orientation?: "horizontal" | "vertical";
  className?: string;
}

export function Stepper({
  steps,
  orientation = "horizontal",
  className = "",
}: StepperProps) {
  if (orientation === "vertical") {
    return (
      <ol className={`space-y-4 ${className}`}>
        {steps.map((step, idx) => {
          const isLast = idx === steps.length - 1;
          const dotColor =
            step.status === "complete"
              ? "bg-success ring-4 ring-accent-success/10"
              : step.status === "current"
                ? "bg-action-primary ring-4 ring-primary-50"
                : step.status === "failed"
                  ? "bg-danger ring-4 ring-accent-danger/10"
                  : "bg-border-strong";

          return (
            <li
              key={step.id}
              className="relative flex gap-4"
              aria-current={step.status === "current" ? "step" : undefined}
            >
              {!isLast && (
                <div
                  className="absolute left-2.5 top-5 -bottom-2 w-0.5 bg-border-subtle"
                  aria-hidden="true"
                />
              )}
              <div className="relative flex h-5 w-5 shrink-0 items-center justify-center">
                <span className={`h-2.5 w-2.5 rounded-full ${dotColor}`} />
              </div>
              <div className="pt-0.5 pb-2">
                <p className="text-xs font-semibold text-text-primary">
                  {step.title}
                </p>
                {step.description && (
                  <p className="text-xs text-text-secondary mt-0.5">
                    {step.description}
                  </p>
                )}
              </div>
            </li>
          );
        })}
      </ol>
    );
  }

  // Horizontal Stepper (e.g., Cart -> Checkout -> Payment -> Success)
  return (
    <nav aria-label="Progress" className={className}>
      <ol className="flex items-center justify-between w-full">
        {steps.map((step, idx) => {
          const isComplete = step.status === "complete";
          const isCurrent = step.status === "current";
          const isLast = idx === steps.length - 1;

          return (
            <li
              key={step.id}
              aria-current={isCurrent ? "step" : undefined}
              className={`relative flex-1 ${!isLast ? "pr-4 sm:pr-8" : ""}`}
            >
              <div className="flex items-center gap-2">
                <span
                  className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold transition ${
                    isComplete
                      ? "bg-action-primary text-text-inverse"
                      : isCurrent
                        ? "border-2 border-action-primary text-action-primary bg-surface-card"
                        : "border-2 border-border-subtle text-text-disabled bg-surface-card"
                  }`}
                >
                  {isComplete ? "✓" : idx + 1}
                </span>
                <div className="sr-only sm:not-sr-only">
                  <span
                    className={`text-xs font-medium ${
                      isCurrent
                        ? "text-action-primary font-semibold"
                        : isComplete
                          ? "text-text-primary"
                          : "text-text-disabled"
                    }`}
                  >
                    {step.title}
                  </span>
                </div>
              </div>
              {!isLast && (
                <div
                  className="absolute top-3.5 right-0 left-12 -z-10 hidden sm:block h-0.5 bg-border-subtle"
                  aria-hidden="true"
                />
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
