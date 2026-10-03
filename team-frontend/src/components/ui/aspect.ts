/** Fixed aspect ratios shared by Image and Skeleton (CLS = 0, no arbitrary values). */
export type Aspect = "square" | "video" | "4/3" | "3/4" | "2/1";

export const aspectClass: Record<Aspect, string> = {
  square: "aspect-square",
  video: "aspect-video",
  "4/3": "aspect-4/3",
  "3/4": "aspect-3/4",
  "2/1": "aspect-2/1",
};
