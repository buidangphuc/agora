/**
 * Tier 3 status tones shared by Alert, Tag, Badge, Result and Statistic.
 * Tints use the Tier 1 accent scale with opacity (Tier 2 aliases are plain
 * var(...) colours and cannot take an opacity modifier).
 */
export type Tone =
  | "neutral"
  | "primary"
  | "info"
  | "success"
  | "warning"
  | "danger";

export interface ToneStyle {
  /** Tinted surface + border + readable text, for chips and banners. */
  soft: string;
  /** Foreground colour for an icon or heading in this tone. */
  icon: string;
}

export const toneStyles: Record<Tone, ToneStyle> = {
  neutral: {
    soft: "bg-surface-page text-text-secondary border-border-subtle",
    icon: "text-text-secondary",
  },
  primary: {
    soft: "bg-primary-50 text-action-primary border-primary-200",
    icon: "text-action-primary",
  },
  info: {
    soft: "bg-surface-muted text-text-primary border-border-strong",
    icon: "text-text-secondary",
  },
  success: {
    soft: "bg-accent-success/10 text-accent-success-dark border-accent-success/30",
    icon: "text-accent-success-dark",
  },
  warning: {
    soft: "bg-accent-promo/10 text-accent-promo-dark border-accent-promo/40",
    icon: "text-accent-promo-dark",
  },
  danger: {
    soft: "bg-accent-danger/10 text-danger border-accent-danger/30",
    icon: "text-danger",
  },
};
