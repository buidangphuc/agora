import { primitives, semanticNames } from "./index.mjs";

const colors = Object.fromEntries(
  semanticNames().map((name) => [name, `var(--color-${name})`]),
);

const preset = {
  theme: {
    colors: { transparent: "transparent", current: "currentColor", ...colors },
    spacing: primitives.space,
    borderRadius: primitives.radius,
    fontSize: primitives.fontSize,
    fontFamily: primitives.fontFamily,
  },
};

export default preset;
