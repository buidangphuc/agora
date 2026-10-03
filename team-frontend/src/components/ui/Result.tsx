import React from "react";
import { Button } from "./Button";

export type ResultStatus = "success" | "error" | "info" | "warning" | "404";

export interface ResultProps {
  status: ResultStatus;
  title: React.ReactNode;
  subTitle?: React.ReactNode;
  extra?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
}

const statusIcons: Record<
  ResultStatus,
  { icon: string; bg: string; text: string }
> = {
  success: {
    icon: "✓",
    bg: "bg-emerald-50 text-emerald-600 border border-emerald-200",
    text: "text-emerald-600",
  },
  error: {
    icon: "✕",
    bg: "bg-rose-50 text-rose-600 border border-rose-200",
    text: "text-rose-600",
  },
  warning: {
    icon: "!",
    bg: "bg-amber-50 text-amber-600 border border-amber-200",
    text: "text-amber-600",
  },
  info: {
    icon: "ℹ",
    bg: "bg-blue-50 text-blue-600 border border-blue-200",
    text: "text-blue-600",
  },
  "404": {
    icon: "404",
    bg: "bg-gray-100 text-gray-600 border border-gray-200",
    text: "text-gray-600",
  },
};

/**
 * Ant Design-style Result pattern.
 * Used for post-action status: Order Placed, Payment Failed, KYC Submission, 404.
 */
export function Result({
  status,
  title,
  subTitle,
  extra,
  children,
  className = "",
}: ResultProps) {
  const currentStatus = statusIcons[status];

  return (
    <section
      className={`mx-auto max-w-xl text-center py-12 px-6 ${className}`}
      aria-live="polite"
    >
      {/* Status Icon */}
      <div className="flex justify-center mb-5">
        <div
          className={`flex h-18 w-18 items-center justify-center rounded-full text-2xl font-black shadow-xs ${currentStatus.bg}`}
        >
          {currentStatus.icon}
        </div>
      </div>

      {/* Title & Subtitle */}
      <h2 className="text-xl font-bold text-gray-900 tracking-tight sm:text-2xl">
        {title}
      </h2>

      {subTitle && (
        <p className="mt-2 text-xs sm:text-sm text-gray-500 max-w-md mx-auto leading-relaxed">
          {subTitle}
        </p>
      )}

      {/* Embedded Details Content */}
      {children && (
        <div className="mt-6 text-left rounded-xl bg-gray-50/80 p-5 border border-gray-200/80">
          {children}
        </div>
      )}

      {/* Action Buttons */}
      {extra && <div className="mt-8 flex justify-center gap-3">{extra}</div>}
    </section>
  );
}
