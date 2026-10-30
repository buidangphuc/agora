import { components } from "@fe-platform/tokens";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { card } = components;

export type CardProps = NativeProps<"div"> & {
  padded?: boolean;
  hoverable?: boolean;
  loading?: boolean;
};

/** Surface that groups related content such as a product, a summary or a form. */
export function Card({
  padded = true,
  hoverable = false,
  loading = false,
  children,
  ...rest
}: CardProps) {
  return (
    <div
      {...rest}
      aria-busy={loading || undefined}
      className={cx(
        card.base,
        padded && !loading && card.padded,
        hoverable && card.hoverable,
      )}
    >
      {loading ? (
        <div className={card.loading}>
          <div className={card.skeleton} />
          <div className={card.skeleton} />
          <div className={card.skeleton} />
        </div>
      ) : (
        children
      )}
    </div>
  );
}

/** Top row of a flush Card: title on the left, optional actions on the right. */
export function CardHeader(props: NativeProps<"div">) {
  return <div {...props} className={card.header} />;
}

/** Heading of a Card, rendered as an h3. */
export function CardTitle({ children, ...rest }: NativeProps<"h3">) {
  return (
    <h3 {...rest} className={card.title}>
      {children}
    </h3>
  );
}

/** Supporting line under a CardTitle. */
export function CardDescription(props: NativeProps<"p">) {
  return <p {...props} className={card.description} />;
}

/** Padded body of a flush Card. */
export function CardContent(props: NativeProps<"div">) {
  return <div {...props} className={card.content} />;
}

/** Muted bottom row of a flush Card for meta text or links. */
export function CardFooter(props: NativeProps<"div">) {
  return <div {...props} className={card.footer} />;
}
