import { primitives, semantic } from "./tokens.mjs";

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

/**
 * @param {Record<string, string>} values
 * @param {string} indent
 */
function declarations(values, indent) {
  return Object.entries(values)
    .map(([name, value]) => `${indent}--color-${name}: ${value};`)
    .join("\n");
}

export function toCss() {
  const light = resolveTheme("light");
  const dark = resolveTheme("dark");
  return `${[
    `:root {\n${declarations(light, "  ")}\n}`,
    `:root[data-theme="dark"] {\n${declarations(dark, "  ")}\n}`,
    `@media (prefers-color-scheme: dark) {\n  :root:not([data-theme="light"]) {\n${declarations(dark, "    ")}\n  }\n}`,
  ].join("\n\n")}\n`;
}
