import { components } from "@fe-platform/tokens";
import { ImageClient } from "./ImageClient";
import { cx } from "./cx";

const { avatar } = components;

export type AvatarProps = {
  src?: string;
  name?: string;
  size?: "sm" | "md" | "lg" | "xl";
  shape?: "circle" | "square";
  anonymousLabel?: string;
};

/** Up to two initials of a name, for example "Nguyễn Văn A" gives "NA"; "?" when empty. */
export function initials(name?: string): string {
  const words = (name ?? "").trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "?";
  const first = words[0][0];
  const last = words.length > 1 ? words[words.length - 1][0] : "";
  return (first + last).toUpperCase();
}

/** Person or shop picture that falls back to initials when the src is missing or fails. */
export function Avatar({
  src,
  name,
  size = "md",
  shape = "circle",
  anonymousLabel = "User",
}: AvatarProps) {
  const text = <span className={avatar.initials}>{initials(name)}</span>;
  return (
    <span
      role={src ? undefined : "img"}
      aria-label={src ? undefined : name || anonymousLabel}
      className={cx(avatar.base, avatar.shape[shape], avatar.size[size])}
    >
      {src ? (
        <ImageClient src={src} alt={name ?? ""} fallback={text} />
      ) : (
        <span aria-hidden="true">{text}</span>
      )}
    </span>
  );
}
