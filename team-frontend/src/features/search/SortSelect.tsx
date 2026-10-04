"use client";

import { useRouter } from "next/navigation";
import { useTransition } from "react";

import { Select, type SelectOption } from "@/components/ui/Select";

/**
 * Sort `Select` (a client leaf: choosing an option navigates). `hrefs` maps each
 * option value to its URL, so the component holds no URL logic. `aria-busy`
 * while the navigation is pending.
 */
export function SortSelect({
  options,
  hrefs,
  value,
  placeholder,
  label,
  className = "",
}: {
  options: SelectOption[];
  hrefs: Record<string, string>;
  /** Current option value, or "" when none of these options is active. */
  value: string;
  placeholder?: string;
  label: string;
  className?: string;
}) {
  const router = useRouter();
  const [pending, start] = useTransition();

  return (
    <div className={className}>
      <Select
        aria-label={label}
        aria-busy={pending ? "true" : undefined}
        selectSize="md"
        options={options}
        placeholder={placeholder}
        value={value}
        onChange={(e) => {
          const href = hrefs[e.target.value];
          if (href) start(() => router.push(href));
        }}
      />
    </div>
  );
}
