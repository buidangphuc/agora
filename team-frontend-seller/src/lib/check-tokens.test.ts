// @vitest-environment node
import { describe, expect, it } from "vitest";

import { scanSource } from "../../scripts/check-tokens.mjs";

const tokens = (src: string) => scanSource(src).map((v) => v.token);

describe("token lint matcher", () => {
  it("accepts token-based class names", () => {
    const good = `
      export const A = () => (
        <div className="bg-action-primary text-text-primary p-4 text-xs rounded-xl hover:bg-primary-600 md:max-w-2xl">
          <a href="#main">skip</a> <span data-testid="x-y" aria-label="ok" />
        </div>
      );`;
    expect(scanSource(good)).toEqual([]);
  });

  it("flags arbitrary values and reports the line and token", () => {
    const bad = `const a = 1;\nconst B = () => <p className="text-[9px] p-4">x</p>;`;
    expect(scanSource(bad)).toEqual([{ line: 2, token: "text-[9px]" }]);
  });

  it("flags arbitrary values behind variants and with calc()", () => {
    expect(
      tokens(`<i className="md:hover:max-w-[1200px] h-[calc(100vh-4rem)]" />`),
    ).toEqual(["md:hover:max-w-[1200px]", "h-[calc(100vh-4rem)]"]);
  });

  it("flags raw hex, rgb() and hsl() literals", () => {
    expect(
      tokens(
        `const c = "#ee4d2d", d = "#fff", e = "rgba(0,0,0,.5)", f = "hsl(10 20% 30%)";`,
      ),
    ).toEqual(["#ee4d2d", "#fff", "rgba(", "hsl("]);
  });

  it("flags an inline style colour, including multi-line style objects", () => {
    const src =
      "<div\n  style={{\n    width: 10,\n    backgroundColor: tint,\n  }}\n/>";
    expect(scanSource(src)).toEqual([
      { line: 4, token: "style backgroundColor:" },
    ]);
  });

  it("allows a style colour that reads a CSS variable", () => {
    expect(
      scanSource(`<div style={{ color: "var(--color-danger)" }} />`),
    ).toEqual([]);
  });

  it("honours tokens-allow on the same line only", () => {
    const src = [
      `<i className="w-[3px]" /> {/* tokens-allow: 3px divider has no token */}`,
      `<i className="w-[5px]" />`,
    ].join("\n");
    expect(scanSource(src)).toEqual([{ line: 2, token: "w-[5px]" }]);
  });

  it("does not treat an empty allow marker as a reason", () => {
    expect(tokens(`<i className="w-[3px]" /> // tokens-allow:`)).toEqual([
      "w-[3px]",
    ]);
  });

  it("ignores comment-only lines", () => {
    expect(
      scanSource(
        "// brand was #ee4d2d, text-[9px] removed\n/* rgb(0,0,0) */\n * #fff",
      ),
    ).toEqual([]);
  });

  it("does not flag array indexing or hex-looking words that are not colours", () => {
    expect(
      scanSource(
        `const x = items[0]; const id = "#12345"; const y = a-[1];`.replace(
          "a-[1]",
          "0",
        ),
      ),
    ).toEqual([]);
  });
});
