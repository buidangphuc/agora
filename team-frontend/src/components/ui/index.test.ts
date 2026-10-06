import { describe, expect, it } from "vitest";

import * as ui from "./index";

// Every component named by the ui-components spec is exported from the barrel.
const expected = [
  "Alert",
  "Avatar",
  "Badge",
  "Breadcrumb",
  "Button",
  "Card",
  "Checkbox",
  "Descriptions",
  "Drawer",
  "Empty",
  "FormItem",
  "Image",
  "Input",
  "Modal",
  "Pagination",
  "PriceTag",
  "Progress",
  "QuantityPicker",
  "Radio",
  "RadioGroup",
  "Rate",
  "Result",
  "Select",
  "Skeleton",
  "Spin",
  "Statistic",
  "Stepper",
  "Table",
  "Tabs",
  "Tag",
  "Timeline",
  "ToastProvider",
];

describe("ui barrel", () => {
  it.each(expected)("exports %s", (name) => {
    expect((ui as Record<string, unknown>)[name]).toBeDefined();
  });

  it("does not leak the internal useDialog hook", () => {
    expect((ui as Record<string, unknown>).useDialog).toBeUndefined();
  });
});
