import React from "react";
import { type Tone, toneStyles } from "./tones";

export type AlertType = "info" | "success" | "warning" | "error";

export interface AlertViewProps {
  type: AlertType;
  title?: React.ReactNode;
  description?: React.ReactNode;
  /** Optional call to action (retry button, link). */
  action?: React.ReactNode;
  /** Pre-rendered close button (client variant only). */
  closeButton?: React.ReactNode;
  className?: string;
}

const toneFor: Record<AlertType, Tone> = {
  info: "info",
  success: "success",
  warning: "warning",
  error: "danger",
};

const glyph: Record<AlertType, string> = {
  info: "i",
  success: "✓",
  warning: "!",
  error: "✕",
};

/** Markup shared by the server Alert and its closable client island. */
export function AlertView({
  type,
  title,
  description,
  action,
  closeButton,
  className = "",
}: AlertViewProps) {
  const tone = toneStyles[toneFor[type]];
  // Errors interrupt (role=alert); the others are polite status messages.
  const Wrapper = type === "error" ? "div" : "output";
  return (
    <Wrapper
      role={type === "error" ? "alert" : undefined}
      data-type={type}
      className={`flex items-start gap-3 rounded-xl border p-3 ${tone.soft} ${className}`}
    >
      <span
        aria-hidden="true"
        className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-current text-xs font-bold ${tone.icon}`}
      >
        {glyph[type]}
      </span>
      <div className="min-w-0 flex-1 text-sm text-text-primary">
        {title && <p className="font-semibold">{title}</p>}
        {description && (
          <div className={title ? "mt-0.5 text-text-secondary" : ""}>
            {description}
          </div>
        )}
      </div>
      {action && <div className="shrink-0">{action}</div>}
      {closeButton}
    </Wrapper>
  );
}
