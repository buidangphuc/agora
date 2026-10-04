import React from "react";
import { toneStyles } from "./tones";

export type ResultStatus =
  | "success"
  | "error"
  | "info"
  | "warning"
  | "403"
  | "404";

export interface ResultProps {
  status: ResultStatus;
  title: React.ReactNode;
  subTitle?: React.ReactNode;
  extra?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
}

// Tier 3: component tokens (tones shared with Alert/Tag).
const statusIcons: Record<
  ResultStatus,
  { icon: string; bg: string; label: string }
> = {
  success: {
    icon: "✓",
    bg: `${toneStyles.success.soft} border`,
    label: "Thành công",
  },
  error: {
    icon: "✕",
    bg: `${toneStyles.danger.soft} border`,
    label: "Lỗi",
  },
  warning: {
    icon: "!",
    bg: `${toneStyles.warning.soft} border`,
    label: "Cảnh báo",
  },
  info: {
    icon: "i",
    bg: `${toneStyles.info.soft} border`,
    label: "Thông tin",
  },
  "403": {
    icon: "403",
    bg: `${toneStyles.danger.soft} border`,
    label: "Không có quyền truy cập",
  },
  "404": {
    icon: "404",
    bg: `${toneStyles.neutral.soft} border`,
    label: "Không tìm thấy",
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
          role="img"
          aria-label={currentStatus.label}
          className={`flex h-18 w-18 items-center justify-center rounded-full text-2xl font-black shadow-xs ${currentStatus.bg}`}
        >
          {currentStatus.icon}
        </div>
      </div>

      {/* Title & Subtitle */}
      <h2 className="text-xl font-bold text-text-primary tracking-tight sm:text-2xl">
        {title}
      </h2>

      {subTitle && (
        <p className="mt-2 text-xs sm:text-sm text-text-secondary max-w-md mx-auto leading-relaxed">
          {subTitle}
        </p>
      )}

      {/* Embedded Details Content */}
      {children && (
        <div className="mt-6 text-left rounded-xl bg-surface-muted p-5 border border-border-subtle">
          {children}
        </div>
      )}

      {/* Action Buttons */}
      {extra && <div className="mt-8 flex justify-center gap-3">{extra}</div>}
    </section>
  );
}
