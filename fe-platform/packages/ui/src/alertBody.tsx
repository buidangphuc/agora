import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { cx } from "./cx";

const { alert } = components;

export type AlertTone = keyof typeof alert.tone;

export type AlertBodyProps = {
  tone: AlertTone;
  title?: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  "data-testid"?: string;
};

const glyph: Record<AlertTone, string> = {
  info: "i",
  success: "✓",
  warning: "!",
  danger: "✕",
};

export function AlertBody({
  tone,
  title,
  description,
  action,
  "data-testid": testId,
  close,
}: AlertBodyProps & { close?: ReactNode }) {
  const danger = tone === "danger";
  const Live = danger ? "div" : "output";
  return (
    <Live
      role={danger ? "alert" : undefined}
      data-tone={tone}
      data-testid={testId}
      className={cx(alert.root, alert.tone[tone])}
    >
      <span aria-hidden="true" className={cx(alert.icon, alert.iconTone[tone])}>
        {glyph[tone]}
      </span>
      <div className={alert.body}>
        {title && <p className={alert.title}>{title}</p>}
        {description && (
          <div className={title ? alert.description : alert.message}>
            {description}
          </div>
        )}
      </div>
      {action && <div className={alert.action}>{action}</div>}
      {close}
    </Live>
  );
}
