import {
  Image as PlatformImage,
  type ImageProps as PlatformImageProps,
} from "@fe-platform/ui";

export type ImageProps = Omit<PlatformImageProps, "fallbackLabel">;

export function Image(props: ImageProps) {
  return <PlatformImage {...props} fallbackLabel="Không có ảnh" />;
}
