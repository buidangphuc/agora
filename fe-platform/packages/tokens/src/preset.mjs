import { primitives, semanticNames, shadowNames } from "./index.mjs";
import { px } from "./resolve.mjs";

const colors = Object.fromEntries(
  semanticNames().map((name) => [name, `var(--color-${name})`]),
);

/** @param {Record<string, number>} scale */
function pxScale(scale) {
  return Object.fromEntries(Object.entries(scale).map(([k, v]) => [k, px(v)]));
}

/**
 * @param {{ addUtilities: (u: Record<string, Record<string, string>>) => void }} api
 */
function safeArea({ addUtilities }) {
  addUtilities({
    ".pt-safe": { paddingTop: "env(safe-area-inset-top)" },
    ".pb-safe": { paddingBottom: "env(safe-area-inset-bottom)" },
  });
}

const preset = {
  theme: {
    colors: { transparent: "transparent", current: "currentColor", ...colors },
    spacing: pxScale(primitives.space),
    borderRadius: pxScale(primitives.radius),
    fontSize: Object.fromEntries(
      Object.entries(primitives.fontSize).map(([k, [size, lineHeight]]) => [
        k,
        [px(size), px(lineHeight)],
      ]),
    ),
    fontFamily: primitives.fontFamily,
    screens: pxScale(primitives.breakpoint),
    minHeight: { touch: px(primitives.touchTarget) },
    extend: {
      aspectRatio: { "4/3": "4 / 3", "3/4": "3 / 4", "2/1": "2 / 1" },
    },
    boxShadow: {
      none: "none",
      ...Object.fromEntries(
        shadowNames().map((n) => [n, `var(--shadow-${n})`]),
      ),
    },
  },
  plugins: [safeArea],
};

export default preset;
