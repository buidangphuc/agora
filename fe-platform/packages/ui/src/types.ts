import type { ComponentPropsWithRef } from "react";

export type NativeProps<T extends keyof React.JSX.IntrinsicElements> = Omit<
  ComponentPropsWithRef<T>,
  "className" | "style"
>;
