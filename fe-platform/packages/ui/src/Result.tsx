import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import type { Tone } from "./Tag";
import { cx } from "./cx";

const { result, tone: tones } = components;

export type ResultStatus =
  | "success"
  | "error"
  | "info"
  | "warning"
  | "403"
  | "404";

const statuses: Record<
  ResultStatus,
  { icon: string; tone: Tone; label: string }
> = {
  success: { icon: "✓", tone: "success", label: "Success" },
  error: { icon: "✕", tone: "danger", label: "Error" },
  warning: { icon: "!", tone: "warning", label: "Warning" },
  info: { icon: "i", tone: "info", label: "Information" },
  "403": { icon: "403", tone: "danger", label: "Access denied" },
  "404": { icon: "404", tone: "neutral", label: "Not found" },
};

export type ResultProps = {
  status: ResultStatus;
  title: ReactNode;
  statusLabel?: string;
  subTitle?: ReactNode;
  extra?: ReactNode;
  children?: ReactNode;
  "data-testid"?: string;
};

/** Outcome screen after an action or for a missing page: status icon, title, details and next actions. */
export function Result({
  status,
  title,
  statusLabel,
  subTitle,
  extra,
  children,
  "data-testid": testId,
}: ResultProps) {
  const s = statuses[status];
  return (
    <section className={result.root} aria-live="polite" data-testid={testId}>
      <div className={result.iconRow}>
        <div
          role="img"
          aria-label={statusLabel ?? s.label}
          className={cx(result.icon, tones[s.tone])}
        >
          {s.icon}
        </div>
      </div>
      <h2 className={result.title}>{title}</h2>
      {subTitle && <p className={result.subTitle}>{subTitle}</p>}
      {children && <div className={result.details}>{children}</div>}
      {extra && <div className={result.extra}>{extra}</div>}
    </section>
  );
}
