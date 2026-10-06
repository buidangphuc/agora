import React from "react";
import { type Tone, toneStyles } from "./tones";

export type TagColor = Tone;

export interface TagViewProps extends React.HTMLAttributes<HTMLSpanElement> {
  color?: TagColor;
  /** Pre-rendered close button (client variant only). */
  closeButton?: React.ReactNode;
}

/** Markup shared by the server Tag and its closable client island. */
export function TagView({
  color = "neutral",
  closeButton,
  className = "",
  children,
  ...props
}: TagViewProps) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-lg border px-2 py-0.5 text-xs font-medium leading-4 ${toneStyles[color].soft} ${className}`}
      {...props}
    >
      {children}
      {closeButton}
    </span>
  );
}
