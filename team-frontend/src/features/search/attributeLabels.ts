/**
 * Display labels for the dynamic (classifier) facets. The contract carries only a
 * facet group and a tag slug, so labels are derived here: a Vietnamese title per
 * known group (fallback: the group id) and the slug read back as words.
 */
const GROUP_LABELS: Record<string, string> = {
  connectivity: "Kết nối",
  feature: "Tính năng",
  material: "Chất liệu",
  display: "Màn hình",
  capacity: "Dung lượng",
  ram: "RAM",
  power: "Công suất",
  color: "Màu sắc",
  size: "Kích cỡ",
  style: "Kiểu dáng",
  usage: "Nhu cầu sử dụng",
  edition: "Phiên bản",
  general: "Khác",
};

export function attributeGroupLabel(group: string): string {
  return GROUP_LABELS[group] ?? group;
}

/** "bluetooth-5-3" -> "Bluetooth 5 3"; "512gb" -> "512gb". */
export function attributeSlugLabel(slug: string): string {
  const words = slug.replace(/-/g, " ").trim();
  return words ? words.charAt(0).toUpperCase() + words.slice(1) : slug;
}

/** `sku.color` -> { prefix: "sku", group: "color" }. */
export function splitAttrKey(key: string): { prefix: string; group: string } {
  const dot = key.indexOf(".");
  return { prefix: key.slice(0, dot), group: key.slice(dot + 1) };
}
