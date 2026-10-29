/** @type {{ fg: string, bg: string, size: string }[]} */
export const pairs = [
  ...["surface-page", "surface-card", "surface-muted"].flatMap((bg) => [
    { fg: "text-primary", bg, size: "normal" },
    { fg: "text-secondary", bg, size: "normal" },
  ]),
  { fg: "text-inverse", bg: "action-primary", size: "normal" },
  { fg: "text-inverse", bg: "action-primary-hover", size: "normal" },
  ...["text-danger", "text-success", "text-warning"].flatMap((fg) =>
    ["surface-card", "surface-page"].map((bg) => ({ fg, bg, size: "normal" })),
  ),
  { fg: "border-danger", bg: "surface-card", size: "ui" },
  { fg: "border-danger", bg: "surface-page", size: "ui" },
  { fg: "focus-ring", bg: "surface-page", size: "ui" },
  { fg: "focus-ring", bg: "surface-card", size: "ui" },
];
