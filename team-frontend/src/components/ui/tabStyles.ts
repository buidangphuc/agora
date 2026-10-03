// Tier 3: component tokens for Tabs, shared by the client and link variants.
export type TabsVariant = "underline" | "pills";

export const tabStyles: Record<
  TabsVariant,
  {
    list: string;
    tab: string;
    active: string;
    inactive: string;
    badgeActive: string;
    badgeInactive: string;
  }
> = {
  underline: {
    list: "-mb-px flex space-x-6 overflow-x-auto",
    tab: "py-3 px-1 border-b-2 font-medium text-xs whitespace-nowrap transition duration-150 cursor-pointer flex items-center gap-1.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 rounded-xs",
    active: "border-action-primary text-action-primary font-semibold",
    inactive:
      "border-transparent text-text-secondary hover:text-text-primary hover:border-border-strong",
    badgeActive: "bg-primary-100 text-action-primary font-bold",
    badgeInactive: "bg-surface-page text-text-secondary",
  },
  pills: {
    list: "flex gap-1.5 p-1 bg-surface-page rounded-lg",
    tab: "px-3 py-1.5 text-xs font-medium rounded-lg transition duration-150 cursor-pointer flex items-center gap-1.5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring",
    active: "bg-surface-card text-text-primary shadow-2xs font-semibold",
    inactive: "text-text-secondary hover:text-text-primary",
    badgeActive: "bg-primary-50 text-action-primary",
    badgeInactive: "bg-border-subtle text-text-secondary",
  },
};

export const tabBadge = "text-xs px-1.5 py-0.5 rounded-full";
