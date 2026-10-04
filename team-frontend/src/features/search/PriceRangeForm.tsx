"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";

/** Validation of the custom price range; returns the message for the max field. */
export function priceRangeError(min: string, max: string): string | undefined {
  const lo = min.trim();
  const hi = max.trim();
  if (lo !== "" && !(Number(lo) >= 0)) return "Giá không hợp lệ.";
  if (hi !== "" && !(Number(hi) >= 0)) return "Giá không hợp lệ.";
  if (lo !== "" && hi !== "" && Number(lo) > Number(hi)) {
    return "Giá tối đa phải lớn hơn hoặc bằng giá tối thiểu.";
  }
  return undefined;
}

/**
 * Custom price range as a GET form: works without JavaScript (hidden inputs keep
 * the other params), and with it validates first and never navigates on an
 * invalid range. `hidden` carries the params to preserve (q, sort, filters).
 */
export function PriceRangeForm({
  id,
  hidden,
  minPrice,
  maxPrice,
  showSubmit = true,
}: {
  id: string;
  hidden: Record<string, string>;
  minPrice?: number;
  maxPrice?: number;
  /** The mobile drawer renders its own sticky submit button (`form={id}`). */
  showSubmit?: boolean;
}) {
  const router = useRouter();
  const [min, setMin] = useState(
    minPrice !== undefined ? String(minPrice) : "",
  );
  const [max, setMax] = useState(
    maxPrice !== undefined ? String(maxPrice) : "",
  );
  const [error, setError] = useState<string | undefined>();

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    const problem = priceRangeError(min, max);
    setError(problem);
    e.preventDefault();
    if (problem) return;
    const params = new URLSearchParams(hidden);
    if (min.trim() !== "" && Number(min) > 0)
      params.set("minPrice", min.trim());
    if (max.trim() !== "" && Number(max) > 0)
      params.set("maxPrice", max.trim());
    const qs = params.toString();
    router.push(qs ? `/search?${qs}` : "/search");
  }

  return (
    <form
      id={id}
      method="get"
      action="/search"
      onSubmit={handleSubmit}
      noValidate
      className="space-y-3"
      aria-label="Khoảng giá tự nhập"
    >
      {Object.entries(hidden).map(([name, value]) => (
        <input key={name} type="hidden" name={name} value={value} />
      ))}
      <div className="grid grid-cols-2 gap-2">
        <FormItem label="Từ (₫)">
          <Input
            type="number"
            name="minPrice"
            inputMode="numeric"
            min={0}
            inputSize="sm"
            value={min}
            onChange={(e) => setMin(e.target.value)}
          />
        </FormItem>
        <FormItem
          label="Đến (₫)"
          status={error ? "error" : undefined}
          help={error}
        >
          <Input
            type="number"
            name="maxPrice"
            inputMode="numeric"
            min={0}
            inputSize="sm"
            value={max}
            onChange={(e) => setMax(e.target.value)}
          />
        </FormItem>
      </div>
      {showSubmit && (
        <Button type="submit" variant="primary" size="md" className="w-full">
          Áp dụng
        </Button>
      )}
    </form>
  );
}
