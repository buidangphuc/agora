import {
  Empty as PlatformEmpty,
  type EmptyProps as PlatformEmptyProps,
} from "@fe-platform/ui";

export interface EmptyProps extends Omit<PlatformEmptyProps, "description"> {
  description?: PlatformEmptyProps["description"];
}

/** Ant Design `Empty`: illustration, description and optional action. Server-compatible. */
export function Empty({
  description = "Không có dữ liệu",
  ...props
}: EmptyProps) {
  return <PlatformEmpty {...props} description={description} />;
}
