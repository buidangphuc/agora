export interface ShopTheme {
  key: "default" | "dark" | "festive";
  headerBg: string;
  headerText: string;
  accent: string;
}

export const SHOP_THEMES: Record<ShopTheme["key"], ShopTheme> = {
  default: {
    key: "default",
    headerBg: "bg-primary-700",
    headerText: "text-white",
    accent: "text-primary-700",
  },
  dark: {
    key: "dark",
    headerBg: "bg-neutral-900",
    headerText: "text-white",
    accent: "text-neutral-900",
  },
  festive: {
    key: "festive",
    headerBg: "bg-danger-dark",
    headerText: "text-white",
    accent: "text-danger-dark",
  },
};

export function resolveShopTheme(key: string | null | undefined): ShopTheme {
  if (key && Object.prototype.hasOwnProperty.call(SHOP_THEMES, key)) {
    return SHOP_THEMES[key as ShopTheme["key"]];
  }
  return SHOP_THEMES.default;
}
