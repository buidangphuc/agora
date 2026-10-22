import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

interface Param {
  key?: string;
  value?: string;
  list?: { map?: Param[] }[];
}
interface Variable {
  name?: string;
  type?: string;
  parameter?: Param[];
}
interface Tag {
  name?: string;
  type?: string;
  parameter?: Param[];
}

const PATH = resolve(__dirname, "../../../../analytics/gtm-container.json");
const data = JSON.parse(readFileSync(PATH, "utf-8"));
const version = data.containerVersion ?? data;
const variables = (version.variable ?? []) as Variable[];
const tags = (version.tag ?? []) as Tag[];

const dlvNamed = (n: string) =>
  variables.filter(
    (v) =>
      v.type === "v" &&
      v.parameter?.some((p) => p.key === "name" && p.value === n),
  );

describe("GTM container maps page_location", () => {
  it("[page_location has no returnUrl] a Data Layer Variable for page_location exists", () => {
    expect(dlvNamed("page_location").length).toBeGreaterThanOrEqual(1);
  });

  it("[page_location has no returnUrl] GA4 Configuration (gaawc) fieldsToSet maps page_location to that variable", () => {
    const vars = dlvNamed("page_location").map((v) => `{{${v.name}}}`);
    const cfg = tags.find((t) => t.name === "GA4 Configuration");
    expect(cfg?.type).toBe("gaawc");
    const fields = cfg?.parameter?.find((p) => p.key === "fieldsToSet");
    const entry = fields?.list
      ?.map((l) => l.map ?? [])
      .find((m) =>
        m.some((p) => p.key === "fieldName" && p.value === "page_location"),
      );
    expect(entry).toBeDefined();
    const value = entry?.find((p) => p.key === "value")?.value;
    expect(vars).toContain(value);
  });
});
