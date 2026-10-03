import Link from "next/link";
import React from "react";
import { TabsClient } from "./TabsClient";
import { type TabsVariant, tabBadge, tabStyles } from "./tabStyles";

export interface TabItem {
  id: string;
  label: string;
  badge?: string | number;
  /** Panel content (client variant only); renders a role="tabpanel" for this tab. */
  content?: React.ReactNode;
  disabled?: boolean;
}

export interface TabsProps {
  items: TabItem[];
  activeId?: string;
  defaultActiveId?: string;
  onChange?: (id: string) => void;
  variant?: TabsVariant;
  className?: string;
  /**
   * Link variant: when given, every tab is a link to `hrefFor(id)` and the tab
   * state lives in the URL (searchParams). Rendered without client JavaScript,
   * so it can be used from a server component; `activeId` marks the current
   * tab with `aria-current="page"`. `onChange`, `defaultActiveId` and
   * `content` do not apply to this variant.
   */
  hrefFor?: (id: string) => string;
}

/**
 * Ant Design `Tabs`. With `hrefFor` it is a server-compatible navigation list;
 * without it, a client island (TabsClient) with arrow-key navigation.
 * This file intentionally has no "use client" so the link variant can receive
 * the `hrefFor` function from a server component.
 */
export function Tabs({
  items,
  activeId,
  defaultActiveId,
  onChange,
  variant = "underline",
  className = "",
  hrefFor,
}: TabsProps) {
  if (!hrefFor) {
    return (
      <TabsClient
        items={items}
        activeId={activeId}
        defaultActiveId={defaultActiveId}
        onChange={onChange}
        variant={variant}
        className={className}
      />
    );
  }

  const styles = tabStyles[variant];
  const nav = (
    <nav aria-label="Tabs">
      <ul className={styles.list}>
        {items.map((tab) => {
          const isActive = tab.id === activeId;
          return (
            <li key={tab.id}>
              <Link
                href={hrefFor(tab.id)}
                aria-current={isActive ? "page" : undefined}
                className={`${styles.tab} ${isActive ? styles.active : styles.inactive}`}
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
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );

  return (
    <div className={className}>
      {variant === "underline" ? (
        <div className="border-b border-border-subtle">{nav}</div>
      ) : (
        nav
      )}
    </div>
  );
}
