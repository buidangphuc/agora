import React from "react";
import { AlertClient } from "./AlertClient";
import { AlertView, type AlertViewProps } from "./alertView";

export type { AlertType } from "./alertView";

export interface AlertProps extends Omit<AlertViewProps, "closeButton"> {
  /** Adds a close button (turns the alert into a client island). */
  closable?: boolean;
  onClose?: () => void;
}

/**
 * Ant Design `Alert` (info / success / warning / error) with an optional
 * action. Server-compatible unless `closable`; errors use role="alert".
 */
export function Alert({ closable = false, onClose, ...view }: AlertProps) {
  if (closable) return <AlertClient {...view} onClose={onClose} />;
  return <AlertView {...view} />;
}
