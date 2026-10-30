import { components } from "@fe-platform/tokens";
import { cx } from "./cx";
import { formatPrice } from "./format";

const { priceTag } = components;

export type PriceTagProps = {
  price: number;
  originalPrice?: number;
  size?: "sm" | "md" | "lg" | "xl";
  showCurrencySymbol?: boolean;
};

/** Price in dong with an optional struck-through original price and discount badge. */
export function PriceTag({
  price,
  originalPrice,
  size = "md",
  showCurrencySymbol = true,
}: PriceTagProps) {
  const sized = priceTag.size[size];
  const discounted = originalPrice !== undefined && originalPrice > price;
  const percent = discounted
    ? Math.round(((originalPrice - price) / originalPrice) * 100)
    : 0;
  return (
    <div className={priceTag.root}>
      <div className={priceTag.price}>
        {showCurrencySymbol && (
          <span className={cx(priceTag.symbol, sized.symbol)}>₫</span>
        )}
        <span className={sized.price}>
          {showCurrencySymbol
            ? formatPrice(price).replace("₫", "").trim()
            : formatPrice(price)}
        </span>
      </div>
      {discounted && (
        <span className={cx(priceTag.original, sized.original)}>
          {formatPrice(originalPrice)}
        </span>
      )}
      {percent > 0 && (
        <span className={cx(priceTag.discount, sized.discount)}>
          -{percent}%
        </span>
      )}
    </div>
  );
}
