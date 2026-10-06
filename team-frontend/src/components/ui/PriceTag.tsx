import React from "react";
import { formatPrice } from "./format";

export interface PriceTagProps {
  price: number;
  originalPrice?: number;
  size?: "sm" | "md" | "lg" | "xl";
  className?: string;
  showCurrencySymbol?: boolean;
}

const sizeMap = {
  sm: {
    symbol: "text-xs",
    price: "text-xs font-bold",
    original: "text-xs",
    discount: "text-xs px-1",
  },
  md: {
    symbol: "text-xs",
    price: "text-sm font-bold",
    original: "text-xs",
    discount: "text-xs px-1",
  },
  lg: {
    symbol: "text-sm",
    price: "text-lg font-bold",
    original: "text-xs",
    discount: "text-xs px-1.5",
  },
  xl: {
    symbol: "text-lg",
    price: "text-2xl font-black",
    original: "text-sm",
    discount: "text-xs px-2",
  },
};

export function PriceTag({
  price,
  originalPrice,
  size = "md",
  className = "",
  showCurrencySymbol = true,
}: PriceTagProps) {
  const currentSize = sizeMap[size];

  // Calculate discount percentage if original price is greater than current
  const discountPercent =
    originalPrice && originalPrice > price
      ? Math.round(((originalPrice - price) / originalPrice) * 100)
      : null;

  return (
    <div
      className={`inline-flex items-baseline gap-1.5 flex-wrap ${className}`}
    >
      {/* Current Sale Price */}
      <div className="text-action-primary flex items-baseline tracking-tight">
        {showCurrencySymbol && (
          <span className={`font-semibold mr-0.5 ${currentSize.symbol}`}>
            ₫
          </span>
        )}
        <span className={currentSize.price}>
          {showCurrencySymbol
            ? formatPrice(price).replace("₫", "").trim()
            : formatPrice(price)}
        </span>
      </div>

      {/* Strikethrough Original Price */}
      {originalPrice && originalPrice > price && (
        <span
          className={`text-text-disabled line-through font-normal ${currentSize.original}`}
        >
          {formatPrice(originalPrice)}
        </span>
      )}

      {/* Discount Badge */}
      {discountPercent !== null && discountPercent > 0 && (
        <span
          className={`bg-primary-50 text-action-primary rounded-xs font-bold leading-tight ${currentSize.discount}`}
        >
          -{discountPercent}%
        </span>
      )}
    </div>
  );
}
