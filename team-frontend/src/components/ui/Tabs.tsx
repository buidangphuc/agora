"use client";

import React, { useState } from "react";

export interface TabItem {
  id: string;
  label: string;
  badge?: string | number;
}

export interface TabsProps {
  items: TabItem[];
  activeId?: string;
  defaultActiveId?: string;
  onChange?: (id: string) => void;
  variant?: "underline" | "pills";
  className?: string;
}

export function Tabs({
  items,
  activeId,
  defaultActiveId,
  onChange,
  variant = "underline",
  className = "",
}: TabsProps) {
  const [internalActive, setInternalActive] = useState(
    activeId || defaultActiveId || items[0]?.id || "",
  );

  const currentActive = activeId !== undefined ? activeId : internalActive;

  const handleSelect = (id: string) => {
    setInternalActive(id);
    onChange?.(id);
  };

  if (variant === "pills") {
    return (
      <div className={`flex gap-1.5 p-1 bg-gray-100 rounded-lg ${className}`}>
        {items.map((tab) => {
          const isActive = tab.id === currentActive;
          return (
            <button
              key={tab.id}
              type="button"
              onClick={() => handleSelect(tab.id)}
              className={`px-3 py-1.5 text-xs font-medium rounded-md transition cursor-pointer flex items-center gap-1.5 ${
                isActive
                  ? "bg-white text-gray-900 shadow-2xs font-semibold"
                  : "text-gray-500 hover:text-gray-700"
              }`}
            >
              <span>{tab.label}</span>
              {tab.badge !== undefined && (
                <span
                  className={`text-xs px-1.5 py-0.2 rounded-full ${
                    isActive
                      ? "bg-primary-50 text-primary-600"
                      : "bg-gray-200 text-gray-600"
                  }`}
                >
                  {tab.badge}
                </span>
              )}
            </button>
          );
        })}
      </div>
    );
  }

  // Underline variant
  return (
    <div className={`border-b border-gray-200 ${className}`}>
      <nav className="-mb-px flex space-x-6 overflow-x-auto" aria-label="Tabs">
        {items.map((tab) => {
          const isActive = tab.id === currentActive;
          return (
            <button
              key={tab.id}
              type="button"
              onClick={() => handleSelect(tab.id)}
              className={`py-3 px-1 border-b-2 font-medium text-xs whitespace-nowrap transition cursor-pointer flex items-center gap-1.5 ${
                isActive
                  ? "border-primary-500 text-primary-600 font-semibold"
                  : "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300"
              }`}
            >
              <span>{tab.label}</span>
              {tab.badge !== undefined && (
                <span
                  className={`text-xs px-1.5 py-0.2 rounded-full ${
                    isActive
                      ? "bg-primary-100 text-primary-600 font-bold"
                      : "bg-gray-100 text-gray-600"
                  }`}
                >
                  {tab.badge}
                </span>
              )}
            </button>
          );
        })}
      </nav>
    </div>
  );
}
