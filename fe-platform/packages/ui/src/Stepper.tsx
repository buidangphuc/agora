import { components } from "@fe-platform/tokens";
import NextLink from "next/link";
import { cx } from "./cx";

const { stepper, timeline } = components;

export type StepStatus = keyof typeof stepper.markerStatus;

export type StepItem = {
  id: string | number;
  title: string;
  description?: string;
  status: StepStatus;
  href?: string;
  disabled?: boolean;
};

export type StepperProps = {
  steps: StepItem[];
  orientation?: "horizontal" | "vertical";
  label?: string;
};

function StepTitle({ step }: { step: StepItem }) {
  const title = cx(stepper.title, stepper.titleStatus[step.status]);
  if (step.href && step.disabled) {
    return (
      <span aria-disabled="true" className={cx(title, stepper.inert)}>
        {step.title}
      </span>
    );
  }
  if (step.href) {
    return (
      <NextLink href={step.href} className={cx(title, stepper.link)}>
        {step.title}
      </NextLink>
    );
  }
  return <span className={title}>{step.title}</span>;
}

/** Progress through ordered steps; the current step carries `aria-current="step"`. Horizontal shows numbered markers (titles from sm up, optional links back to earlier steps); vertical shows a dotted list with descriptions. */
export function Stepper({
  steps,
  orientation = "horizontal",
  label = "Progress",
}: StepperProps) {
  if (orientation === "vertical") {
    return (
      <ol className={timeline.list}>
        {steps.map((step, idx) => (
          <li
            key={step.id}
            className={timeline.item}
            aria-current={step.status === "current" ? "step" : undefined}
          >
            {idx < steps.length - 1 && (
              <div aria-hidden="true" className={timeline.line} />
            )}
            <div className={timeline.dotWrap}>
              <span
                aria-hidden="true"
                className={cx(timeline.dot, stepper.dotStatus[step.status])}
              />
            </div>
            <div className={timeline.body}>
              <p className={cx(timeline.title, timeline.titleDefault)}>
                {step.title}
              </p>
              {step.description && (
                <p className={timeline.description}>{step.description}</p>
              )}
            </div>
          </li>
        ))}
      </ol>
    );
  }

  return (
    <nav aria-label={label}>
      <ol className={stepper.list}>
        {steps.map((step, idx) => {
          const isLast = idx === steps.length - 1;
          return (
            <li
              key={step.id}
              aria-current={step.status === "current" ? "step" : undefined}
              className={isLast ? stepper.last : stepper.item}
            >
              <span
                className={cx(
                  stepper.marker,
                  stepper.markerStatus[step.status],
                )}
              >
                {step.status === "complete" ? "✓" : idx + 1}
              </span>
              <span className={stepper.label}>
                <StepTitle step={step} />
              </span>
              {!isLast && (
                <span aria-hidden="true" className={stepper.connector} />
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
