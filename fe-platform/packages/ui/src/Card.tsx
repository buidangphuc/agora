import { components } from "@fe-platform/tokens";
import type { NativeProps } from "./types";

const { card } = components;

/** Surface that groups related content such as a product, a summary or a form. */
export function Card(props: NativeProps<"div">) {
  return <div {...props} className={card.base} />;
}
