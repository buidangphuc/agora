import {
  Alert as PlatformAlert,
  type AlertProps as PlatformAlertProps,
} from "@fe-platform/ui";

export type AlertType = "info" | "success" | "warning" | "error";

export interface AlertProps
  extends Omit<PlatformAlertProps, "tone" | "closeLabel"> {
  type: AlertType;
}

const toneFor = {
  info: "info",
  success: "success",
  warning: "warning",
  error: "danger",
} as const;

export function Alert({ type, ...rest }: AlertProps) {
  return <PlatformAlert {...rest} tone={toneFor[type]} closeLabel="Đóng" />;
}
