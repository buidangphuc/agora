import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { components } from "@fe-platform/tokens";
import postcss from "postcss";
import tailwindcss from "tailwindcss";
import { describe, expect, it } from "vitest";
import config from "./tailwind.config.mjs";

const here = dirname(fileURLToPath(import.meta.url));

function classesOf(node) {
  if (typeof node === "string") return node.split(/\s+/).filter(Boolean);
  return Object.values(node).flatMap(classesOf);
}

function escapeClass(name) {
  return name.replace(/[:/[\]().%]/g, (c) => `\\${c}`);
}

async function generate(content) {
  const result = await postcss([tailwindcss({ ...config, content })]).process(
    "@tailwind utilities;",
    { from: undefined },
  );
  return result.css;
}

const absolute = config.content.map((g) => resolve(here, g));
const expected = [...new Set(classesOf(components))];

describe("tailwind content covers the component class maps", () => {
  it("generates CSS for every class in packages/tokens components", async () => {
    const css = await generate(absolute);
    const missing = expected.filter((c) => !css.includes(`.${escapeClass(c)}`));
    expect(missing).toEqual([]);
  });

  it("goes red when the token class maps are not scanned", async () => {
    const withoutTokens = absolute.filter(
      (g) => !g.includes("packages/tokens"),
    );
    const css = await generate(withoutTokens);
    const missing = expected.filter((c) => !css.includes(`.${escapeClass(c)}`));
    expect(missing.length).toBeGreaterThan(0);
  });
});
