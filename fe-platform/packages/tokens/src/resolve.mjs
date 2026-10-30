import { primitives, semantic, shadow } from "./tokens.mjs";

export const THEMES = ["light", "dark"];

/**
 * @param {string} theme
 * @returns {Record<string, string>}
 */
export function resolveTheme(theme) {
  /** @type {Record<string, string>} */
  const out = {};
  for (const [name, ref] of Object.entries(
    /** @type {Record<string, Record<string, string>>} */ (semantic)[theme],
  )) {
    const value = /** @type {Record<string, string>} */ (primitives.color)[ref];
    if (!value) throw new Error(`${theme}.${name}: unknown primitive "${ref}"`);
    out[name] = value;
  }
  return out;
}

export function semanticNames() {
  return Object.keys(semantic.light);
}

export function px(/** @type {number} */ n) {
  return n === 0 ? "0px" : `${n}px`;
}

/**
 * @param {{ x: number, y: number, blur: number, spread: number, color: string }[]} layers
 */
export function shadowToCss(layers) {
  return layers
    .map(
      (l) => `${px(l.x)} ${px(l.y)} ${px(l.blur)} ${px(l.spread)} ${l.color}`,
    )
    .join(", ");
}

/**
 * @param {string} theme
 * @returns {Record<string, string>}
 */
export function resolveShadows(theme) {
  const layers = /** @type {Record<string, Record<string, any[]>>} */ (shadow)[
    theme
  ];
  return Object.fromEntries(
    Object.entries(layers).map(([name, l]) => [name, shadowToCss(l)]),
  );
}

export function shadowNames() {
  return Object.keys(shadow.light);
}

/**
 * @param {string} theme
 * @param {string} indent
 */
function declarations(theme, indent) {
  const colors = Object.entries(resolveTheme(theme)).map(
    ([name, value]) => `${indent}--color-${name}: ${value};`,
  );
  const shadows = Object.entries(resolveShadows(theme)).map(
    ([name, value]) => `${indent}--shadow-${name}: ${value};`,
  );
  return [...colors, ...shadows].join("\n");
}

export function toCss() {
  return `${[
    `:root {\n${declarations("light", "  ")}\n}`,
    `:root[data-theme="dark"] {\n${declarations("dark", "  ")}\n}`,
    `@media (prefers-color-scheme: dark) {\n  :root:not([data-theme="light"]) {\n${declarations("dark", "    ")}\n  }\n}`,
  ].join("\n\n")}\n`;
}

export function toJson() {
  const fontSize = Object.fromEntries(
    Object.entries(primitives.fontSize).map(([k, [size, lineHeight]]) => [
      k,
      { size, lineHeight },
    ]),
  );
  return `${JSON.stringify(
    {
      color: { light: resolveTheme("light"), dark: resolveTheme("dark") },
      shadow,
      space: primitives.space,
      radius: primitives.radius,
      fontSize,
      fontFamily: primitives.fontFamily,
      breakpoint: primitives.breakpoint,
      touchTarget: primitives.touchTarget,
    },
    null,
    2,
  )}\n`;
}
