import { components } from "@fe-platform/tokens";
import NextLink from "next/link";
import type { ReactNode } from "react";
import { TabsClient } from "./TabsClient";
import { cx } from "./cx";

const { tabs } = components;

export type TabsVariant = keyof typeof tabs.variant;

export type TabItem = {
  id: string;
  label: string;
  badge?: string | number;
  content?: ReactNode;
  disabled?: boolean;
};

export type TabsProps = {
  items: TabItem[];
  activeId?: string;
  defaultActiveId?: string;
  onChange?: (id: string) => void;
  variant?: TabsVariant;
  hrefFor?: (id: string) => string;
  label?: string;
};

/** Switches between views. With `hrefFor` every tab is a server-rendered link and the URL holds the state; without it a client tablist with arrow keys and panels. */
export function Tabs({
  items,
  activeId,
  defaultActiveId,
  onChange,
  variant = "underline",
  hrefFor,
  label = "Tabs",
}: TabsProps) {
  if (!hrefFor) {
    return (
      <TabsClient
        items={items}
        activeId={activeId}
        defaultActiveId={defaultActiveId}
        onChange={onChange}
        variant={variant}
      />
    );
  }
  const style = tabs.variant[variant];
  const nav = (
    <nav aria-label={label}>
      <ul className={style.list}>
        {items.map((tab) => {
          const isActive = tab.id === activeId;
          return (
            <li key={tab.id}>
              <NextLink
                href={hrefFor(tab.id)}
                aria-current={isActive ? "page" : undefined}
                className={cx(
                  tabs.tab,
                  style.tab,
                  isActive ? style.active : style.inactive,
                )}
              >
                <span>{tab.label}</span>
                {tab.badge !== undefined && (
                  <span
                    className={cx(
                      tabs.badge,
                      isActive ? style.badgeActive : style.badgeInactive,
                    )}
                  >
                    {tab.badge}
                  </span>
                )}
              </NextLink>
            </li>
          );
        })}
      </ul>
    </nav>
  );
  return variant === "underline" ? (
    <div className={tabs.frame}>{nav}</div>
  ) : (
    nav
  );
}
