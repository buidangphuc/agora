import {
  Card as PlatformCard,
  type CardProps as PlatformCardProps,
} from "@fe-platform/ui";

export {
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@fe-platform/ui";

export type CardProps = PlatformCardProps;

export function Card({ padded = false, ...props }: CardProps) {
  return <PlatformCard padded={padded} {...props} />;
}
