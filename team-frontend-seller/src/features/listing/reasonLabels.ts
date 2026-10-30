const REASON_LABELS: Record<string, string> = {
  prohibited_item: "Hàng cấm",
  counterfeit: "Hàng giả",
  misleading: "Thông tin gây hiểu lầm",
  inappropriate: "Nội dung không phù hợp",
  duplicate: "Tin trùng lặp",
  other: "Khác",
};

/** Label of a moderation reason code; an unknown code is shown raw. */
export function reasonLabel(code: string): string {
  return REASON_LABELS[code] ?? code;
}
