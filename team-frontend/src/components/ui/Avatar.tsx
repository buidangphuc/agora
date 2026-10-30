import {
  Avatar as PlatformAvatar,
  type AvatarProps as PlatformAvatarProps,
} from "@fe-platform/ui";

export { initials } from "@fe-platform/ui";

export type AvatarProps = Omit<PlatformAvatarProps, "anonymousLabel">;

export function Avatar(props: AvatarProps) {
  return <PlatformAvatar {...props} anonymousLabel="Người dùng" />;
}
