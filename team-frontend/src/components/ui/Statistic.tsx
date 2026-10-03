import React from "react";
import { Card } from "./Card";

export interface StatisticProps {
  title: React.ReactNode;
  value: React.ReactNode;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  trend?: {
    value: number | string;
    isUp?: boolean;
    label?: string;
  };
  className?: string;
}

/**
 * Ant Design-style Statistic pattern.
 * Displays executive metrics, revenue counters, and KPI stats for merchant center.
 */
export function Statistic({
  title,
  value,
  prefix,
  suffix,
  trend,
  className = "",
}: StatisticProps) {
  return (
    <Card
      className={`p-5 rounded-2xl border-gray-200/80 shadow-preline-card ${className}`}
    >
      <span className="text-xs font-medium text-gray-500 block">{title}</span>

      <div className="mt-2 flex items-baseline gap-1 text-2xl sm:text-3xl font-black text-gray-900 tracking-tight">
        {prefix && (
          <span className="text-lg font-bold text-gray-400 mr-1">{prefix}</span>
        )}
        <span>{value}</span>
        {suffix && (
          <span className="text-sm font-semibold text-gray-500 ml-1">
            {suffix}
          </span>
        )}
      </div>

      {trend && (
        <div className="mt-2.5 flex items-center gap-1.5 text-xs">
          <span
            className={`font-semibold flex items-center gap-0.5 ${
              trend.isUp ? "text-emerald-600" : "text-rose-600"
            }`}
          >
            <span>{trend.isUp ? "↑" : "↓"}</span>
            <span>{trend.value}</span>
          </span>
          {trend.label && <span className="text-gray-400">{trend.label}</span>}
        </div>
      )}
    </Card>
  );
}
