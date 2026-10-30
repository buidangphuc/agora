import { Drawer as PlatformDrawer } from "@fe-platform/ui";
import type React from "react";

export interface DrawerProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  description?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
  placement?: "left" | "right";
  size?: "sm" | "md" | "lg";
}

export function Drawer({ isOpen, ...rest }: DrawerProps) {
  return (
    <PlatformDrawer
      {...rest}
      open={isOpen}
      label="Ngăn kéo"
      closeLabel="Đóng"
    />
  );
}
