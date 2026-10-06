import React, { useId } from "react";

export type FormItemStatus = "error" | "warning" | "success";

export interface FormItemProps {
  label?: React.ReactNode;
  /** Shows the required mark (visual only; set `required` on the control too). */
  required?: boolean;
  /** Help or validation message under the control. */
  help?: React.ReactNode;
  /** Validation status: colours the help text and sets `aria-invalid` on `error`. */
  status?: FormItemStatus;
  /**
   * Group mode (RadioGroup, Rate): the label is not a `<label>`; it names the
   * child through `aria-labelledby` instead.
   */
  group?: boolean;
  /** Single control (it receives id / aria-describedby / aria-invalid) or any content. */
  children: React.ReactNode;
  className?: string;
}

// Tier 3: component tokens.
const helpTone: Record<FormItemStatus, string> = {
  error: "text-danger font-medium",
  warning: "text-accent-promo-dark",
  success: "text-accent-success-dark",
};

/**
 * Ant Design `Form.Item`: label, required mark, help text and validation
 * status, wired to the control with htmlFor / aria-describedby / aria-invalid.
 * Server-compatible. For Input, which carries its own label, use its props.
 */
export function FormItem({
  label,
  required = false,
  help,
  status,
  group = false,
  children,
  className = "",
}: FormItemProps) {
  const baseId = useId();
  const labelId = `${baseId}-label`;
  const helpId = `${baseId}-help`;
  const only = React.isValidElement(children) ? children : null;
  const childProps = (only?.props ?? {}) as Record<string, unknown>;
  const controlId =
    (childProps.id as string | undefined) ?? `${baseId}-control`;
  const describedBy =
    [childProps["aria-describedby"], help ? helpId : undefined]
      .filter(Boolean)
      .join(" ") || undefined;

  const control = only
    ? React.cloneElement(only as React.ReactElement<Record<string, unknown>>, {
        ...(group ? { "aria-labelledby": labelId } : { id: controlId }),
        "aria-describedby": describedBy,
        "aria-invalid":
          status === "error" ? true : (childProps["aria-invalid"] ?? undefined),
      })
    : children;

  const mark = required && (
    <span className="ml-0.5 text-danger" aria-hidden="true">
      *
    </span>
  );
  const labelClass = "block text-xs font-medium text-text-primary";

  return (
    <div className={`space-y-1.5 ${className}`} data-status={status}>
      {label &&
        (group || !only ? (
          <span id={labelId} className={labelClass}>
            {label}
            {mark}
          </span>
        ) : (
          <label htmlFor={controlId} className={labelClass}>
            {label}
            {mark}
          </label>
        ))}
      {control}
      {help && (
        <p
          id={helpId}
          aria-live={status === "error" ? "polite" : undefined}
          className={`text-xs ${status ? helpTone[status] : "text-text-secondary"}`}
        >
          {help}
        </p>
      )}
    </div>
  );
}
