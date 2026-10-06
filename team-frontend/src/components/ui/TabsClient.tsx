"use client";

import React, { useId, useRef, useState } from "react";
import type { TabItem } from "./Tabs";
import { type TabsVariant, tabBadge, tabStyles } from "./tabStyles";

export interface TabsClientProps {
  items: TabItem[];
  activeId?: string;
  defaultActiveId?: string;
  onChange?: (id: string) => void;
  variant: TabsVariant;
  className: string;
}

/** Client-side tabs: tablist/tab/tabpanel roles, roving tabindex, arrow keys. */
export function TabsClient({
  items,
  activeId,
  defaultActiveId,
  onChange,
  variant,
  className,
}: TabsClientProps) {
  const baseId = useId();
  const [internalActive, setInternalActive] = useState(
    activeId || defaultActiveId || items[0]?.id || "",
  );
  const tabRefs = useRef(new Map<string, HTMLButtonElement>());
  const currentActive = activeId !== undefined ? activeId : internalActive;
  const styles = tabStyles[variant];

  const select = (id: string) => {
    setInternalActive(id);
    onChange?.(id);
  };

  const onKeyDown = (e: React.KeyboardEvent, index: number) => {
    const enabled = items.filter((t) => !t.disabled);
    if (enabled.length === 0) return;
    const position = enabled.findIndex((t) => t.id === items[index].id);
    let target: TabItem | undefined;
    if (e.key === "ArrowRight") {
      target = enabled[(position + 1) % enabled.length];
    } else if (e.key === "ArrowLeft") {
      target = enabled[(position - 1 + enabled.length) % enabled.length];
    } else if (e.key === "Home") {
      target = enabled[0];
    } else if (e.key === "End") {
      target = enabled[enabled.length - 1];
    }
    if (!target) return;
    e.preventDefault();
    select(target.id);
    tabRefs.current.get(target.id)?.focus();
  };

  const tabId = (id: string) => `${baseId}-tab-${id}`;
  const panelId = (id: string) => `${baseId}-panel-${id}`;

  const list = (
    <div role="tablist" className={styles.list}>
      {items.map((tab, index) => {
        const isActive = tab.id === currentActive;
        return (
          <button
            key={tab.id}
            ref={(node) => {
              if (node) tabRefs.current.set(tab.id, node);
              else tabRefs.current.delete(tab.id);
            }}
            id={tabId(tab.id)}
            type="button"
            role="tab"
            aria-selected={isActive}
            aria-controls={
              tab.content !== undefined ? panelId(tab.id) : undefined
            }
            aria-disabled={tab.disabled ? "true" : undefined}
            disabled={tab.disabled}
            tabIndex={isActive ? 0 : -1}
            onClick={() => select(tab.id)}
            onKeyDown={(e) => onKeyDown(e, index)}
            className={`${styles.tab} disabled:opacity-50 disabled:pointer-events-none disabled:cursor-not-allowed ${
              isActive ? styles.active : styles.inactive
            }`}
          >
            <span>{tab.label}</span>
            {tab.badge !== undefined && (
              <span
                className={`${tabBadge} ${
                  isActive ? styles.badgeActive : styles.badgeInactive
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

  return (
    <div className={className}>
      {variant === "underline" ? (
        <div className="border-b border-border-subtle">{list}</div>
      ) : (
        list
      )}
      {items.map(
        (tab) =>
          tab.content !== undefined && (
            <div
              key={tab.id}
              id={panelId(tab.id)}
              role="tabpanel"
              aria-labelledby={tabId(tab.id)}
              hidden={tab.id !== currentActive}
              className="pt-4"
            >
              {tab.content}
            </div>
          ),
      )}
    </div>
  );
}
